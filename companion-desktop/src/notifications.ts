// notifications.ts — the three notification policies carried over from
// companion-desktop/notify.go (docs/companion-api-surface.md ops #34-36).
// Pure decision logic, kept separate from Electron's Notification API so
// it can be unit tested without a display. main.ts feeds it consecutive
// /api/state snapshots (from polling or the SSE `event: changed` nudge)
// plus window-visibility, and gets back a list of notifications to fire.

export interface WorldSnapshot {
  worldId: string;
  worldName: string;
  // Custody state as reported by the engine snapshot for this world.
  holder?: string | null;
  claimedBy?: string | null;
  expiresAt?: string | null; // ISO timestamp of the current hold's expiry
}

export interface StateSnapshot {
  links: WorldSnapshot[];
  sync: {
    lastError?: string | null;
  };
}

export interface NotifyEvent {
  kind: "claimArrived" | "holdNearlyUp" | "syncFailedUnseen";
  title: string;
  body: string;
  worldId?: string;
}

const HOLD_WARN_WINDOW_MS = 15 * 60 * 1000;
const HOLD_WARN_REFIRE_MS = 60 * 60 * 1000;

/** Mutable debounce/edge-trigger state the policy needs between snapshots. */
export class NotifyPolicyState {
  private holdWarned = new Map<string, number>(); // worldId -> last-fired epoch ms
  private lastError: string | null | undefined = undefined;
  private lastLinksById = new Map<string, WorldSnapshot>();
  private everSeen = false;

  /**
   * Evaluates the three policies against a new snapshot and returns the
   * notifications to fire. `windowHidden` is whether the app window is
   * currently hidden (tray-only) — required for #36.
   */
  evaluate(now: StateSnapshot, windowHidden: boolean, nowMs: number = Date.now()): NotifyEvent[] {
    const events: NotifyEvent[] = [];
    const prevLinksById = this.lastLinksById;

    if (this.everSeen) {
      // #34 claimArrived: a claim appeared on a world that had none before,
      // i.e. someone queued behind a hold unattended.
      for (const world of now.links) {
        const prev = prevLinksById.get(world.worldId);
        const hadClaim = !!prev?.claimedBy;
        const hasClaim = !!world.claimedBy;
        if (!hadClaim && hasClaim) {
          events.push({
            kind: "claimArrived",
            title: "Someone's waiting",
            body: `${world.worldName}: ${world.claimedBy} asked to be next.`,
            worldId: world.worldId,
          });
        }
      }
    }

    // #35 holdNearlyUp: your hold is inside its last 15 minutes. Debounced
    // to fire at most once per hour per world while still inside the window.
    for (const world of now.links) {
      if (!world.expiresAt) continue;
      const expiresMs = Date.parse(world.expiresAt);
      if (Number.isNaN(expiresMs)) continue;
      const remaining = expiresMs - nowMs;
      if (remaining > 0 && remaining <= HOLD_WARN_WINDOW_MS) {
        const lastFired = this.holdWarned.get(world.worldId) ?? 0;
        if (nowMs - lastFired >= HOLD_WARN_REFIRE_MS) {
          this.holdWarned.set(world.worldId, nowMs);
          events.push({
            kind: "holdNearlyUp",
            title: "Hold expiring soon",
            body: `${world.worldName}'s hold expires in under 15 minutes.`,
            worldId: world.worldId,
          });
        }
      } else if (remaining <= 0) {
        // Hold lapsed or was renewed past the window; clear so a future
        // re-entry into the window fires again rather than staying muted.
        this.holdWarned.delete(world.worldId);
      }
    }

    // #36 syncFailedUnseen: edge-triggered on LastError changing to a
    // non-empty value, and only while the window is hidden.
    if (this.everSeen) {
      const errNow = now.sync.lastError || null;
      const errChanged = errNow !== (this.lastError || null);
      if (errChanged && errNow && windowHidden) {
        events.push({
          kind: "syncFailedUnseen",
          title: "Sync failed",
          body: errNow,
        });
      }
    }
    this.lastError = now.sync.lastError ?? null;

    this.lastLinksById = new Map(now.links.map((w) => [w.worldId, w]));
    this.everSeen = true;
    return events;
  }
}
