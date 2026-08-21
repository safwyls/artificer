package savesync

// The custody state machine, exercised against a real sqlite store with
// only the test game registered — the core gate. The invariants under
// test are the ones the architecture doc stakes the feature on: one
// active hold per world, expiry is claimable-not-ended, and only the
// active session moves the head.

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/crypto"
	"github.com/safwyls/artificer/core/db"
	"github.com/safwyls/artificer/core/store"

	_ "github.com/safwyls/artificer/core/game/gametest"
)

func newTestService(t *testing.T) (*Service, *store.Store, *sql.DB) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	box, err := crypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	st := store.New(sqlDB, box)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(st, nil, logger, t.TempDir()), st, sqlDB
}

func makeUser(t *testing.T, st *store.Store, name, role string) *store.User {
	t.Helper()
	id, err := st.CreateUser(context.Background(), name, "x", role, nil)
	if err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	u, err := st.GetUser(context.Background(), id)
	if err != nil {
		t.Fatalf("get user %s: %v", name, err)
	}
	return u
}

// bundle builds a save bundle the way the agent and the companion do: a
// tar of relative regular files.
func bundle(t *testing.T, files map[string]string) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), ModTime: time.Now(), Format: tar.FormatPAX}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	return &buf
}

// goodWorld passes the default 16-byte verification floor.
func goodWorld(t *testing.T) io.Reader {
	return bundle(t, map[string]string{"World.sav": "0123456789abcdef0123456789abcdef"})
}

func makeWorld(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	id, err := st.CreateSyncWorld(context.Background(), name, time.Now())
	if err != nil {
		t.Fatalf("create world: %v", err)
	}
	return id
}

// expireHold ages the active session past its lease, which no store verb
// does on purpose — expiry is only ever the clock's doing.
func expireHold(t *testing.T, sqlDB *sql.DB, sessionID int64) {
	t.Helper()
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if _, err := sqlDB.Exec(`UPDATE sync_sessions SET expires_at = ? WHERE id = ?`, past, sessionID); err != nil {
		t.Fatalf("aging session: %v", err)
	}
}

func TestCheckoutCheckinFastForward(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	wid := makeWorld(t, st, "midgard")

	ss, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if ss.BaseVersion != nil {
		t.Errorf("fresh world checkout has base %v, want nil", *ss.BaseVersion)
	}

	v, err := svc.Checkin(ctx, ss, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if v.Conflict {
		t.Error("first checkin flagged as conflict")
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion == nil || *w.HeadVersion != v.ID {
		t.Errorf("head = %v, want %d", w.HeadVersion, v.ID)
	}
	if _, err := st.ActiveSyncSession(ctx, wid); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("world still held after checkin: %v", err)
	}
	got, _ := st.GetSyncSession(ctx, ss.ID)
	if got.Status != store.SyncReturned {
		t.Errorf("session status = %q, want returned", got.Status)
	}
	if _, err := os.Stat(svc.versionPath(wid, v.ID)); err != nil {
		t.Errorf("version archive missing: %v", err)
	}
}

func TestSecondCheckoutRefused(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	wid := makeWorld(t, st, "midgard")

	if _, err := svc.Checkout(ctx, wid, alice, false, false); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	_, err := svc.Checkout(ctx, wid, bob, false, false)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("second checkout: got %v, want HeldError", err)
	}
	if held.Claimable || held.Holder != "alice" {
		t.Errorf("HeldError = %+v, want unclaimable, held by alice", held)
	}
}

func TestExpiredHoldTakeoverAndLateCheckin(t *testing.T) {
	svc, st, sqlDB := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	wid := makeWorld(t, st, "midgard")

	aliceSS, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	expireHold(t, sqlDB, aliceSS.ID)

	// Expired is claimable, never automatic: without takeover it is
	// still a refusal, with the claimable flag for the client to ask.
	_, err = svc.Checkout(ctx, wid, bob, false, false)
	var held *HeldError
	if !errors.As(err, &held) || !held.Claimable {
		t.Fatalf("expired checkout without takeover: got %v, want claimable HeldError", err)
	}

	bobSS, err := svc.Checkout(ctx, wid, bob, false, true)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if got, _ := st.GetSyncSession(ctx, aliceSS.ID); got.Status != store.SyncReclaimed {
		t.Errorf("old session status = %q, want reclaimed", got.Status)
	}

	// Bob plays and checks in: fast-forward.
	bobV, err := svc.Checkin(ctx, bobSS, bob, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("bob checkin: %v", err)
	}
	if bobV.Conflict {
		t.Error("bob's checkin flagged as conflict")
	}

	// Alice's late check-in from the reclaimed session: kept, flagged,
	// never the head.
	aliceV, err := svc.Checkin(ctx, aliceSS, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("alice late checkin: %v", err)
	}
	if !aliceV.Conflict {
		t.Error("late checkin from a reclaimed session was not flagged")
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion == nil || *w.HeadVersion != bobV.ID {
		t.Errorf("head = %v, want bob's %d", w.HeadVersion, bobV.ID)
	}
}

// A check-in from an ended session never fast-forwards, even when the
// head has not moved: moving it would be under the new holder's feet.
func TestEndedSessionNeverMovesHead(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	admin := makeUser(t, st, "root", store.RoleAdmin)
	wid := makeWorld(t, st, "midgard")

	ss, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if err := svc.Release(ctx, ss, admin); err != nil {
		t.Fatalf("release: %v", err)
	}
	v, err := svc.Checkin(ctx, ss, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("late checkin: %v", err)
	}
	if !v.Conflict {
		t.Error("checkin from a released session was not flagged")
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion != nil {
		t.Errorf("head moved to %d by a released session", *w.HeadVersion)
	}
}

func TestClaimNextHandoff(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	wid := makeWorld(t, st, "midgard")

	aliceSS, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if err := svc.Claim(ctx, wid, bob); err != nil {
		t.Fatalf("claim: %v", err)
	}

	v, err := svc.Checkin(ctx, aliceSS, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("checkin: %v", err)
	}

	// The handoff: bob holds the world now, based on alice's check-in,
	// and the claim queue is empty again.
	active, err := st.ActiveSyncSession(ctx, wid)
	if err != nil {
		t.Fatalf("no active session after handoff: %v", err)
	}
	if active.HolderID != bob.ID {
		t.Errorf("holder = %d, want bob (%d)", active.HolderID, bob.ID)
	}
	if active.BaseVersion == nil || *active.BaseVersion != v.ID {
		t.Errorf("handoff base = %v, want %d", active.BaseVersion, v.ID)
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.NextHolder != nil {
		t.Errorf("claim not cleared after handoff")
	}
}

func TestClaimRules(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	carol := makeUser(t, st, "carol", "")
	wid := makeWorld(t, st, "midgard")

	if err := svc.Claim(ctx, wid, bob); !errors.Is(err, ErrWorldFree) {
		t.Errorf("claim on a free world: got %v, want ErrWorldFree", err)
	}
	if _, err := svc.Checkout(ctx, wid, alice, false, false); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if err := svc.Claim(ctx, wid, bob); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.Claim(ctx, wid, carol); !errors.Is(err, ErrReserved) {
		t.Errorf("second claim: got %v, want ErrReserved", err)
	}
	if err := svc.Unclaim(ctx, wid, carol); !errors.Is(err, ErrReserved) {
		t.Errorf("unclaim of someone else's claim: got %v, want ErrReserved", err)
	}
	if err := svc.Unclaim(ctx, wid, bob); err != nil {
		t.Fatalf("unclaim: %v", err)
	}
	if err := svc.Claim(ctx, wid, carol); err != nil {
		t.Fatalf("claim after unclaim: %v", err)
	}
}

func TestReservedCheckout(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	bob := makeUser(t, st, "bob", "")
	carol := makeUser(t, st, "carol", "")
	wid := makeWorld(t, st, "midgard")

	// A free world with a standing claim (a handoff that could not run,
	// say): the claim reserves the next checkout.
	if err := st.SetSyncWorldNextHolder(ctx, wid, &bob.ID); err != nil {
		t.Fatalf("seeding claim: %v", err)
	}
	if _, err := svc.Checkout(ctx, wid, carol, false, false); !errors.Is(err, ErrReserved) {
		t.Errorf("reserved checkout: got %v, want ErrReserved", err)
	}
	if _, err := svc.Checkout(ctx, wid, bob, false, false); err != nil {
		t.Fatalf("claimant checkout: %v", err)
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.NextHolder != nil {
		t.Error("claim not consumed by the claimant's checkout")
	}
}

func TestCheckpointsAndRetention(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	wid := makeWorld(t, st, "midgard")

	ss, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.Checkin(ctx, ss, alice, goodWorld(t), store.SyncKindCheckpoint); err != nil {
			t.Fatalf("checkpoint %d: %v", i, err)
		}
	}
	// The session stays active and the head never moves.
	if _, err := st.ActiveSyncSession(ctx, wid); err != nil {
		t.Fatalf("session ended by a checkpoint: %v", err)
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion != nil {
		t.Errorf("checkpoint moved the head to %d", *w.HeadVersion)
	}
	// Retention keeps the newest keepCheckpoints of the active session.
	versions, _ := st.ListSyncVersions(ctx, wid)
	if len(versions) != keepCheckpoints {
		t.Errorf("kept %d checkpoints, want %d", len(versions), keepCheckpoints)
	}

	// After the real check-in, the session's checkpoints are prunable
	// and go.
	if _, err := svc.Checkin(ctx, ss, alice, goodWorld(t), store.SyncKindCheckin); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	versions, _ = st.ListSyncVersions(ctx, wid)
	for _, v := range versions {
		if v.Kind == store.SyncKindCheckpoint {
			t.Errorf("checkpoint %d survived its session's end", v.ID)
		}
	}
}

func TestImport(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	wid := makeWorld(t, st, "midgard")

	v, err := svc.Import(ctx, wid, alice, goodWorld(t))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion == nil || *w.HeadVersion != v.ID {
		t.Errorf("import did not set the head")
	}

	if _, err := svc.Checkout(ctx, wid, alice, false, false); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := svc.Import(ctx, wid, alice, goodWorld(t)); err == nil {
		t.Error("import while held was not refused")
	}
}

func TestUploadVerification(t *testing.T) {
	svc, st, _ := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	wid := makeWorld(t, st, "midgard")
	ss, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	var uerr *UploadError
	if _, err := svc.Checkin(ctx, ss, alice, bytes.NewReader([]byte("not a tar")), store.SyncKindCheckin); !errors.As(err, &uerr) {
		t.Errorf("garbage upload: got %v, want UploadError", err)
	}
	// A world below the verification floor is a torn save, not a save.
	torn := bundle(t, map[string]string{"World.sav": "tiny"})
	if _, err := svc.Checkin(ctx, ss, alice, torn, store.SyncKindCheckin); !errors.As(err, &uerr) {
		t.Errorf("torn world: got %v, want UploadError", err)
	}
	// Nothing was committed, and the session still holds the world.
	if versions, _ := st.ListSyncVersions(ctx, wid); len(versions) != 0 {
		t.Errorf("refused uploads left %d versions", len(versions))
	}
	if got, _ := st.GetSyncSession(ctx, ss.ID); got.Status != store.SyncActive {
		t.Errorf("refused upload ended the session: %q", got.Status)
	}
}

func TestPruneKeepsHeadAndConflicts(t *testing.T) {
	svc, st, sqlDB := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	wid := makeWorld(t, st, "midgard")
	if err := st.UpdateSyncWorldSettings(ctx, wid, "midgard", nil, 48, 1<<20, 2, true, ""); err != nil {
		t.Fatalf("settings: %v", err)
	}

	// Manufacture one conflict: alice's expired hold, bob takes over and
	// checks in, alice checks in late.
	aliceSS, _ := svc.Checkout(ctx, wid, alice, false, false)
	expireHold(t, sqlDB, aliceSS.ID)
	bobSS, err := svc.Checkout(ctx, wid, bob, false, true)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if _, err := svc.Checkin(ctx, bobSS, bob, goodWorld(t), store.SyncKindCheckin); err != nil {
		t.Fatalf("bob checkin: %v", err)
	}
	conflictV, err := svc.Checkin(ctx, aliceSS, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("late checkin: %v", err)
	}

	// Churn past the keep window.
	for i := 0; i < 4; i++ {
		ss, err := svc.Checkout(ctx, wid, alice, false, false)
		if err != nil {
			t.Fatalf("churn checkout %d: %v", i, err)
		}
		if _, err := svc.Checkin(ctx, ss, alice, goodWorld(t), store.SyncKindCheckin); err != nil {
			t.Fatalf("churn checkin %d: %v", i, err)
		}
	}

	versions, _ := st.ListSyncVersions(ctx, wid)
	w, _ := st.GetSyncWorld(ctx, wid)
	var checkins int
	var conflictKept, headKept bool
	for _, v := range versions {
		if v.ID == conflictV.ID {
			conflictKept = true
		}
		if w.HeadVersion != nil && v.ID == *w.HeadVersion {
			headKept = true
		}
		if !v.Conflict && v.Kind != store.SyncKindCheckpoint {
			checkins++
		}
	}
	if !conflictKept {
		t.Error("prune removed a conflict version")
	}
	if !headKept {
		t.Error("prune removed the head")
	}
	if checkins > 2 {
		t.Errorf("prune kept %d plain check-ins, want at most keep_versions=2", checkins)
	}
	// The files on disk match the rows.
	for _, v := range versions {
		if _, err := os.Stat(svc.versionPath(wid, v.ID)); err != nil {
			t.Errorf("kept version %d has no archive: %v", v.ID, err)
		}
	}
}

func TestSetHeadResolvesConflicts(t *testing.T) {
	svc, st, sqlDB := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	bob := makeUser(t, st, "bob", "")
	wid := makeWorld(t, st, "midgard")

	aliceSS, _ := svc.Checkout(ctx, wid, alice, false, false)
	expireHold(t, sqlDB, aliceSS.ID)
	bobSS, _ := svc.Checkout(ctx, wid, bob, false, true)
	if _, err := svc.Checkin(ctx, bobSS, bob, goodWorld(t), store.SyncKindCheckin); err != nil {
		t.Fatalf("bob checkin: %v", err)
	}
	conflictV, err := svc.Checkin(ctx, aliceSS, alice, goodWorld(t), store.SyncKindCheckin)
	if err != nil {
		t.Fatalf("late checkin: %v", err)
	}

	// The human picks alice's version after all.
	if err := svc.SetHead(ctx, wid, conflictV.ID); err != nil {
		t.Fatalf("set head: %v", err)
	}
	w, _ := st.GetSyncWorld(ctx, wid)
	if w.HeadVersion == nil || *w.HeadVersion != conflictV.ID {
		t.Errorf("head = %v, want %d", w.HeadVersion, conflictV.ID)
	}
	versions, _ := st.ListSyncVersions(ctx, wid)
	for _, v := range versions {
		if v.Conflict {
			t.Errorf("version %d still flagged after resolve", v.ID)
		}
	}
}

func TestRenewAndExpiryWarning(t *testing.T) {
	svc, st, sqlDB := newTestService(t)
	ctx := context.Background()
	alice := makeUser(t, st, "alice", "")
	wid := makeWorld(t, st, "midgard")

	ss, err := svc.Checkout(ctx, wid, alice, false, false)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	// Move the hold into the warning window and sweep: warned once.
	soon := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	if _, err := sqlDB.Exec(`UPDATE sync_sessions SET expires_at = ? WHERE id = ?`, soon, ss.ID); err != nil {
		t.Fatalf("aging session: %v", err)
	}
	svc.sweep(ctx)
	got, _ := st.GetSyncSession(ctx, ss.ID)
	if got.WarnedAt == nil {
		t.Fatal("sweep did not mark the warning sent")
	}

	// Renew: fresh lease, warning re-armed.
	until, err := svc.Renew(ctx, got)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	got, _ = st.GetSyncSession(ctx, ss.ID)
	if got.WarnedAt != nil {
		t.Error("renew did not re-arm the expiry warning")
	}
	if !got.ExpiresAt.Equal(until.Truncate(time.Second)) && got.ExpiresAt.Before(time.Now().Add(47*time.Hour)) {
		t.Errorf("renew expiry = %v, want ~48h out", got.ExpiresAt)
	}
}
