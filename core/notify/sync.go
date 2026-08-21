package notify

import (
	"context"
	"fmt"
	"time"
)

// Save-sync custody events. Worlds are not servers — a shared world has
// no server row to hang a webhook config on — so each world carries its
// own webhook URL (usually the friend group's channel) and these post to
// it directly, with the same never-ping-anyone mention policy as
// everything else. An empty URL means the world has notifications off;
// callers don't need to check first.

func (n *Notifier) syncPost(ctx context.Context, webhookURL string, e embed) {
	if webhookURL == "" {
		return
	}
	if err := n.post(ctx, webhookURL, e); err != nil {
		n.logger.Warn("notify: save-sync discord delivery failed", "error", err)
	}
}

func (n *Notifier) SyncCheckedOut(ctx context.Context, webhookURL, world, holder string, until time.Time) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "📤 World checked out",
		Description: fmt.Sprintf("**%s** now holds **%s** (until %s).", escapeMarkdown(holder), escapeMarkdown(world), until.UTC().Format("Mon 15:04 MST")),
		Color:       colorBlue,
	})
}

func (n *Notifier) SyncCheckedIn(ctx context.Context, webhookURL, world, holder string) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "📥 World checked in",
		Description: fmt.Sprintf("**%s** checked **%s** back in — the world is free.", escapeMarkdown(holder), escapeMarkdown(world)),
		Color:       colorGreen,
	})
}

// SyncConflict announces a check-in that could not move the head — the
// save is kept and flagged, and a human has to pick. Worth a loud color:
// silence here is how a session quietly goes missing.
func (n *Notifier) SyncConflict(ctx context.Context, webhookURL, world, holder string) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "⚠️ Check-in flagged as a conflict",
		Description: fmt.Sprintf("**%s** checked in a version of **%s** that no longer follows the current head. Both versions are kept; pick one in the console.", escapeMarkdown(holder), escapeMarkdown(world)),
		Color:       colorRed,
	})
}

func (n *Notifier) SyncExpiryWarning(ctx context.Context, webhookURL, world, holder string, expiresAt time.Time) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "⏳ Hold expiring",
		Description: fmt.Sprintf("**%s**'s hold on **%s** expires %s — check in or renew, or the world becomes claimable.", escapeMarkdown(holder), escapeMarkdown(world), expiresAt.UTC().Format("Mon 15:04 MST")),
		Color:       colorAmber,
	})
}

func (n *Notifier) SyncReclaimed(ctx context.Context, webhookURL, world, newHolder, prevHolder string) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "📤 Expired hold claimed",
		Description: fmt.Sprintf("**%s** claimed **%s** from **%s**'s expired hold. A late check-in from the old hold will be kept and flagged, never lost.", escapeMarkdown(newHolder), escapeMarkdown(world), escapeMarkdown(prevHolder)),
		Color:       colorAmber,
	})
}

func (n *Notifier) SyncReleased(ctx context.Context, webhookURL, world, holder, admin string) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "🔓 Hold released",
		Description: fmt.Sprintf("**%s** released **%s**'s hold on **%s**.", escapeMarkdown(admin), escapeMarkdown(holder), escapeMarkdown(world)),
		Color:       colorAmber,
	})
}

// SyncYourTurn is the claim-next handoff: the previous holder checked in
// and the queued claimant's checkout happened automatically.
func (n *Notifier) SyncYourTurn(ctx context.Context, webhookURL, world, holder string, until time.Time) {
	n.syncPost(ctx, webhookURL, embed{
		Title:       "🎮 The world is yours",
		Description: fmt.Sprintf("**%s** is checked out to **%s** (queued claim, until %s) — your companion app will fetch it.", escapeMarkdown(world), escapeMarkdown(holder), until.UTC().Format("Mon 15:04 MST")),
		Color:       colorGreen,
	})
}
