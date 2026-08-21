// Package savesync is the checkout/check-in custody engine for shared
// world saves (docs/save-sync-architecture.md): one holder at a time,
// every check-in a new version, the canonical head moved only by the
// active session or an explicit human pick.
//
// The rules that matter, stated once:
//
//   - The lock is the unique active session row in the store; nothing
//     here holds custody in memory.
//   - Expiry never ends a hold — it makes the hold claimable. Taking
//     over an expired hold is an explicit act, and the previous holder
//     is told.
//   - Only the active session may move the head, and only when its base
//     is the head. Every other check-in is stored and flagged as a
//     conflict, exempt from pruning, until a human picks a head. Data
//     is never silently discarded — that is the whole point.
package savesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/safwyls/artificer/core/game"
	"github.com/safwyls/artificer/core/notify"
	"github.com/safwyls/artificer/core/store"
)

const (
	// sweepEvery paces the expiry-warning sweeper; leases are hours long.
	sweepEvery = time.Minute
	// warnBefore is how far ahead of expiry the warning ping goes out.
	warnBefore = 2 * time.Hour
)

// HeldError says the world is already checked out. Claimable means the
// hold is past its lease, so an explicit takeover would succeed.
type HeldError struct {
	Session   *store.SyncSession
	Holder    string
	Claimable bool
}

func (e *HeldError) Error() string {
	if e.Claimable {
		return fmt.Sprintf("held by %s, expired — claimable", e.Holder)
	}
	return fmt.Sprintf("held by %s until %s", e.Holder, e.Session.ExpiresAt.UTC().Format(time.RFC3339))
}

// ErrReserved: the world's next hold is queued for someone else.
var ErrReserved = errors.New("the next checkout of this world is reserved by another player's claim")

// ErrWorldFree: a claim was made on a world nobody holds.
var ErrWorldFree = errors.New("the world is not checked out — check it out instead of claiming")

type Service struct {
	store    *store.Store
	notifier *notify.Notifier // nil in tests: notifications are best-effort everywhere
	logger   *slog.Logger
	// root is DATA_DIR/savesync; world w's versions live in <root>/<w>/<version>.tar.
	root string

	// mu serializes custody decisions (head moves, session ends, prunes).
	// Transfers stream outside it; only the commit step takes it.
	mu sync.Mutex
	// subscribers is the live-event bus (events.go): custody changes are
	// announced so a watching page needn't poll.
	subscribers
}

func New(st *store.Store, notifier *notify.Notifier, logger *slog.Logger, dataDir string) *Service {
	return &Service{store: st, notifier: notifier, logger: logger, root: filepath.Join(dataDir, "savesync")}
}

// Run sweeps for expiring holds until ctx is cancelled. Intended to be
// started in a goroutine.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep sends the one expiry warning each lease gets. Expiry itself is
// not an event — nothing changes at that moment except what others may
// do — so there is deliberately no "expired!" ping, just the warning
// while the holder can still act on it.
func (s *Service) sweep(ctx context.Context) {
	sessions, err := s.store.ActiveSyncSessions(ctx)
	if err != nil {
		s.logger.Error("savesync: listing active sessions", "error", err)
		return
	}
	now := time.Now()
	for _, ss := range sessions {
		if ss.WarnedAt != nil || now.Add(warnBefore).Before(ss.ExpiresAt) {
			continue
		}
		w, err := s.store.GetSyncWorld(ctx, ss.WorldID)
		if err != nil {
			continue
		}
		if s.notifier != nil {
			s.notifier.SyncExpiryWarning(ctx, w.WebhookURL, w.Name, s.username(ctx, ss.HolderID), ss.ExpiresAt)
		}
		if err := s.store.MarkSyncSessionWarned(ctx, ss.ID, now); err != nil {
			s.logger.Error("savesync: marking session warned", "session", ss.ID, "error", err)
		}
	}
}

// username resolves an id for a notification; a deleted account still
// reads as something.
func (s *Service) username(ctx context.Context, id int64) string {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return fmt.Sprintf("user #%d", id)
	}
	return u.Username
}

// Checkout acquires the world for user. takeover consents to claiming an
// expired hold — without it, an expired hold answers HeldError with
// Claimable set so the client can ask the human first.
func (s *Service) Checkout(ctx context.Context, worldID int64, user *store.User, serverHeld, takeover bool) (*store.SyncSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkoutLocked(ctx, worldID, user, serverHeld, takeover)
}

func (s *Service) checkoutLocked(ctx context.Context, worldID int64, user *store.User, serverHeld, takeover bool) (*store.SyncSession, error) {
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		return nil, err
	}
	if w.NextHolder != nil && *w.NextHolder != user.ID {
		return nil, ErrReserved
	}
	now := time.Now()
	if active, err := s.store.ActiveSyncSession(ctx, worldID); err == nil {
		held := &HeldError{Session: active, Holder: s.username(ctx, active.HolderID), Claimable: active.Expired(now)}
		if !held.Claimable || !takeover {
			return nil, held
		}
		// Explicit takeover of an expired hold. The old session ends as
		// reclaimed — its late check-in will land as a flagged branch, not
		// a lost save — and the old holder is told now, not at check-in.
		if err := s.store.EndSyncSession(ctx, active.ID, store.SyncReclaimed, user.ID, now); err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if s.notifier != nil {
			s.notifier.SyncReclaimed(ctx, w.WebhookURL, w.Name, user.Username, held.Holder)
		}
	}
	id, err := s.store.CreateSyncSession(ctx, worldID, user.ID, serverHeld, w.HeadVersion, now, now.Add(s.lease(w)))
	if errors.Is(err, store.ErrWorldHeld) {
		// Raced another checkout between the read and the insert; the
		// database's index is the arbiter, we just phrase it.
		if active, aerr := s.store.ActiveSyncSession(ctx, worldID); aerr == nil {
			return nil, &HeldError{Session: active, Holder: s.username(ctx, active.HolderID), Claimable: active.Expired(now)}
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if w.NextHolder != nil && *w.NextHolder == user.ID {
		if err := s.store.SetSyncWorldNextHolder(ctx, worldID, nil); err != nil {
			s.logger.Error("savesync: clearing consumed claim", "world", worldID, "error", err)
		}
	}
	ss, err := s.store.GetSyncSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.SyncCheckedOut(ctx, w.WebhookURL, w.Name, user.Username, ss.ExpiresAt)
	}
	s.publish(worldID, EventCheckout)
	return ss, nil
}

func (s *Service) lease(w *store.SyncWorld) time.Duration {
	if w.LeaseHours <= 0 {
		return 48 * time.Hour
	}
	return time.Duration(w.LeaseHours) * time.Hour
}

// Renew extends an active hold by the world's lease from now.
func (s *Service) Renew(ctx context.Context, ss *store.SyncSession) (time.Time, error) {
	w, err := s.store.GetSyncWorld(ctx, ss.WorldID)
	if err != nil {
		return time.Time{}, err
	}
	until := time.Now().Add(s.lease(w))
	if err := s.store.RenewSyncSession(ctx, ss.ID, until); err != nil {
		return time.Time{}, err
	}
	s.publish(ss.WorldID, EventCheckout)
	return until, nil
}

// Claim queues user as the world's next holder. One claimant per world;
// claiming a free world is refused because checkout is the right verb.
func (s *Service) Claim(ctx context.Context, worldID int64, user *store.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		return err
	}
	if _, err := s.store.ActiveSyncSession(ctx, worldID); errors.Is(err, store.ErrNotFound) {
		return ErrWorldFree
	} else if err != nil {
		return err
	}
	if w.NextHolder != nil && *w.NextHolder != user.ID {
		return ErrReserved
	}
	defer s.publish(worldID, EventClaim)
	return s.store.SetSyncWorldNextHolder(ctx, worldID, &user.ID)
}

// Unclaim withdraws a queued claim. Admins may clear anyone's.
func (s *Service) Unclaim(ctx context.Context, worldID int64, user *store.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		return err
	}
	if w.NextHolder == nil {
		return nil
	}
	if *w.NextHolder != user.ID && !user.IsAdmin() {
		return ErrReserved
	}
	defer s.publish(worldID, EventClaim)
	return s.store.SetSyncWorldNextHolder(ctx, worldID, nil)
}

// Release force-ends a hold (admin). The holder's late check-in, if one
// comes, lands as a flagged branch like any check-in from an ended
// session.
func (s *Service) Release(ctx context.Context, ss *store.SyncSession, admin *store.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.EndSyncSession(ctx, ss.ID, store.SyncReleased, admin.ID, time.Now()); err != nil {
		return err
	}
	if w, err := s.store.GetSyncWorld(ctx, ss.WorldID); err == nil && s.notifier != nil {
		s.notifier.SyncReleased(ctx, w.WebhookURL, w.Name, s.username(ctx, ss.HolderID), admin.Username)
	}
	// A release frees the world the same way a check-in does, so a queued
	// claimant gets the same automatic handoff.
	s.handoffToClaimant(ctx, ss.WorldID)
	s.publish(ss.WorldID, EventRelease)
	return nil
}

// Checkin lands an upload as a new version of the session's world.
//
// kind SyncKindCheckin ends the session and — when the session is still
// active and based on the current head — fast-forwards the head; any
// other lineage is stored flagged. kind SyncKindCheckpoint keeps the
// session active and never touches the head. The upload is staged,
// bounded, extracted and verified as a save before any row exists, so a
// dead client or a torn upload leaves at worst a stale staging file.
func (s *Service) Checkin(ctx context.Context, ss *store.SyncSession, user *store.User, body io.Reader, kind string) (*store.SyncVersion, error) {
	w, err := s.store.GetSyncWorld(ctx, ss.WorldID)
	if err != nil {
		return nil, err
	}
	staged, bytes, sum, err := s.stageAndVerify(ctx, w, body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-read both sides of the custody decision: the upload took real
	// time, and an admin may have released the session (or a takeover
	// ended it) while the bytes streamed.
	w, err = s.store.GetSyncWorld(ctx, ss.WorldID)
	if err != nil {
		os.Remove(staged)
		return nil, err
	}
	ss, err = s.store.GetSyncSession(ctx, ss.ID)
	if err != nil {
		os.Remove(staged)
		return nil, err
	}

	now := time.Now()
	v := &store.SyncVersion{
		WorldID:    w.ID,
		SessionID:  &ss.ID,
		ParentID:   ss.BaseVersion,
		Kind:       kind,
		Bytes:      bytes,
		SHA256:     sum,
		UploaderID: &user.ID,
	}
	switch kind {
	case store.SyncKindCheckpoint:
		if ss.Status != store.SyncActive {
			os.Remove(staged)
			return nil, &UploadError{Msg: "this hold has ended — check in instead of checkpointing"}
		}
		if !w.Checkpoints {
			os.Remove(staged)
			return nil, &UploadError{Msg: "checkpoints are off for this world"}
		}
	case store.SyncKindCheckin:
		v.Conflict = ss.Status != store.SyncActive || !ptrEq(ss.BaseVersion, w.HeadVersion)
	default:
		os.Remove(staged)
		return nil, fmt.Errorf("unknown check-in kind %q", kind)
	}

	id, err := s.store.CreateSyncVersion(ctx, v, now)
	if err != nil {
		os.Remove(staged)
		return nil, err
	}
	v.ID, v.CreatedAt = id, now
	if err := os.Rename(staged, s.versionPath(w.ID, id)); err != nil {
		s.store.DeleteSyncVersion(ctx, id)
		os.Remove(staged)
		return nil, err
	}

	if kind == store.SyncKindCheckin {
		if ss.Status == store.SyncActive {
			if err := s.store.EndSyncSession(ctx, ss.ID, store.SyncReturned, user.ID, now); err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		if !v.Conflict {
			if err := s.store.SetSyncWorldHead(ctx, w.ID, id); err != nil {
				return nil, err
			}
		}
		if s.notifier != nil {
			if v.Conflict {
				s.notifier.SyncConflict(ctx, w.WebhookURL, w.Name, user.Username)
			} else {
				s.notifier.SyncCheckedIn(ctx, w.WebhookURL, w.Name, user.Username)
			}
		}
		s.handoffToClaimant(ctx, w.ID)
	}
	s.pruneLocked(ctx, w.ID)
	if kind == store.SyncKindCheckpoint {
		s.publish(w.ID, EventCheckpoint)
	} else {
		s.publish(w.ID, EventCheckin)
	}
	return v, nil
}

// handoffToClaimant is the claim-next payoff: the world just went free,
// so the queued claimant's checkout happens now, without them having to
// notice. Failures log and leave the claim queued — the claimant can
// still check out by hand.
func (s *Service) handoffToClaimant(ctx context.Context, worldID int64) {
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil || w.NextHolder == nil {
		return
	}
	claimant, err := s.store.GetUser(ctx, *w.NextHolder)
	if err != nil {
		s.logger.Warn("savesync: queued claimant no longer exists — clearing the claim", "world", worldID)
		s.store.SetSyncWorldNextHolder(ctx, worldID, nil)
		return
	}
	ss, err := s.checkoutLocked(ctx, worldID, claimant, false, false)
	if err != nil {
		s.logger.Error("savesync: claim-next handoff failed", "world", worldID, "claimant", claimant.Username, "error", err)
		return
	}
	if s.notifier != nil {
		s.notifier.SyncYourTurn(ctx, w.WebhookURL, w.Name, claimant.Username, ss.ExpiresAt)
	}
}

// Import lands an upload as the new head outside any session — seeding a
// fresh world, or a portal upload with the world in hand. Refused while
// the world is held: the holder's checkout is what would be overwritten.
func (s *Service) Import(ctx context.Context, worldID int64, user *store.User, body io.Reader) (*store.SyncVersion, error) {
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		return nil, err
	}
	staged, bytes, sum, err := s.stageAndVerify(ctx, w, body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if active, err := s.store.ActiveSyncSession(ctx, worldID); err == nil {
		os.Remove(staged)
		return nil, &HeldError{Session: active, Holder: s.username(ctx, active.HolderID), Claimable: active.Expired(time.Now())}
	}
	w, err = s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		os.Remove(staged)
		return nil, err
	}
	now := time.Now()
	v := &store.SyncVersion{
		WorldID:    w.ID,
		ParentID:   w.HeadVersion,
		Kind:       store.SyncKindImport,
		Bytes:      bytes,
		SHA256:     sum,
		UploaderID: &user.ID,
	}
	id, err := s.store.CreateSyncVersion(ctx, v, now)
	if err != nil {
		os.Remove(staged)
		return nil, err
	}
	v.ID, v.CreatedAt = id, now
	if err := os.Rename(staged, s.versionPath(w.ID, id)); err != nil {
		s.store.DeleteSyncVersion(ctx, id)
		os.Remove(staged)
		return nil, err
	}
	if err := s.store.SetSyncWorldHead(ctx, w.ID, id); err != nil {
		return nil, err
	}
	s.pruneLocked(ctx, w.ID)
	s.publish(w.ID, EventImport)
	return v, nil
}

// SetHead is the explicit human pick: rollback, or conflict resolution.
// It clears the world's conflict flags — the flag exists to hold
// versions for exactly this decision.
func (s *Service) SetHead(ctx context.Context, worldID, versionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.store.GetSyncVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if v.WorldID != worldID {
		return store.ErrNotFound
	}
	if v.Kind == store.SyncKindCheckpoint {
		// A checkpoint can be promoted — that is what it was saved for —
		// but it is subject to checkpoint pruning until it becomes head,
		// which SetHead itself fixes by protecting the head.
		s.logger.Info("savesync: promoting a checkpoint to head", "world", worldID, "version", versionID)
	}
	if err := s.store.SetSyncWorldHead(ctx, worldID, versionID); err != nil {
		return err
	}
	if err := s.store.ClearSyncConflicts(ctx, worldID); err != nil {
		return err
	}
	s.pruneLocked(ctx, worldID)
	s.publish(worldID, EventHead)
	return nil
}

// stageAndVerify lands the upload in a staging file, then proves it is a
// save: well-formed bundle, within bounds, and a world the game's layout
// recognizes. Returns the staging path for the commit step to rename.
func (s *Service) stageAndVerify(ctx context.Context, w *store.SyncWorld, body io.Reader) (staged string, bytes int64, sum string, err error) {
	staged = filepath.Join(s.root, fmt.Sprintf("%d", w.ID), fmt.Sprintf("staging-%d.tar", time.Now().UnixNano()))
	bytes, sum, err = stageUpload(body, staged, w.MaxBytes)
	if err != nil {
		return "", 0, "", err
	}
	f, err := os.Open(staged)
	if err != nil {
		os.Remove(staged)
		return "", 0, "", err
	}
	verifyDir := staged + ".verify"
	extractErr := extractBundle(f, verifyDir, w.MaxBytes)
	f.Close()
	if extractErr == nil {
		extractErr = verifyExtracted(verifyDir, s.layoutFor(ctx, w))
	}
	os.RemoveAll(verifyDir)
	if extractErr != nil {
		os.Remove(staged)
		return "", 0, "", extractErr
	}
	return staged, bytes, sum, nil
}

// layoutFor resolves the save layout that judges this world's uploads.
// The standalone service registers no game, so this is usually the zero
// value — the permissive structural checks. A game-specific layout comes
// back only where a game module is registered in the hosting binary; the
// generic service cannot know game formats, and pretending otherwise
// would be an invented rule that rejects legitimate saves.
func (s *Service) layoutFor(ctx context.Context, w *store.SyncWorld) game.SaveLayout {
	if def, ok := game.Get(game.DefaultID); ok && def.Save != nil {
		return *def.Save
	}
	return game.SaveLayout{}
}

// VersionPath validates that the version belongs to the world and
// returns its archive for streaming.
func (s *Service) VersionPath(ctx context.Context, worldID, versionID int64) (string, *store.SyncVersion, error) {
	v, err := s.store.GetSyncVersion(ctx, versionID)
	if err != nil {
		return "", nil, err
	}
	if v.WorldID != worldID {
		return "", nil, store.ErrNotFound
	}
	path := s.versionPath(worldID, versionID)
	if _, err := os.Stat(path); err != nil {
		return "", nil, fmt.Errorf("version archive missing from disk: %w", err)
	}
	return path, v, nil
}

func (s *Service) versionPath(worldID, versionID int64) string {
	return filepath.Join(s.root, fmt.Sprintf("%d", worldID), fmt.Sprintf("%d.tar", versionID))
}

// DeleteWorld removes the world, its custody history and its archives.
func (s *Service) DeleteWorld(ctx context.Context, worldID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.DeleteSyncWorld(ctx, worldID); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.root, fmt.Sprintf("%d", worldID))); err != nil {
		s.logger.Error("savesync: removing world archives", "world", worldID, "error", err)
	}
	s.publish(worldID, EventWorld)
	return nil
}

// keepCheckpoints is how many of an active session's checkpoints
// survive pruning — the newest is the crash recovery, one more covers a
// checkpoint that was itself torn by the crash it exists for.
const keepCheckpoints = 2

// pruneLocked applies retention. Never pruned: the head, conflict
// versions (they are waiting on a human), the active session's base, and
// the active session's newest checkpoints. Of what remains, the newest
// keep_versions check-ins/imports stay. A pruned version may be the
// parent of a kept one; lineage display degrades to "parent pruned"
// rather than blocking retention forever.
func (s *Service) pruneLocked(ctx context.Context, worldID int64) {
	w, err := s.store.GetSyncWorld(ctx, worldID)
	if err != nil {
		return
	}
	versions, err := s.store.ListSyncVersions(ctx, worldID) // newest first
	if err != nil {
		s.logger.Error("savesync: listing versions for prune", "world", worldID, "error", err)
		return
	}
	keep := w.KeepVersions
	if keep < 1 {
		keep = 1
	}
	var activeSession *store.SyncSession
	if ss, err := s.store.ActiveSyncSession(ctx, worldID); err == nil {
		activeSession = ss
	}

	kept, checkpoints := 0, 0
	for _, v := range versions {
		protected := v.Conflict ||
			(w.HeadVersion != nil && *w.HeadVersion == v.ID) ||
			(activeSession != nil && activeSession.BaseVersion != nil && *activeSession.BaseVersion == v.ID)
		if v.Kind == store.SyncKindCheckpoint {
			if activeSession != nil && v.SessionID != nil && *v.SessionID == activeSession.ID {
				if checkpoints++; checkpoints <= keepCheckpoints {
					protected = true
				}
			}
		} else if !v.Conflict {
			// The head is almost always among the newest, so it consumes
			// keep quota like any check-in; conflicts are held by their
			// flag alone and never eat into the plain history.
			if kept++; kept <= keep {
				protected = true
			}
		}
		if protected {
			continue
		}
		// Row first, then file: a row whose delete fails keeps its
		// archive, while an orphaned archive is only wasted disk.
		if err := s.store.DeleteSyncVersion(ctx, v.ID); err != nil {
			s.logger.Error("savesync: pruning version row", "world", worldID, "version", v.ID, "error", err)
			continue
		}
		if err := os.Remove(s.versionPath(worldID, v.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Error("savesync: pruning version archive", "world", worldID, "version", v.ID, "error", err)
		}
	}
}

func ptrEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
