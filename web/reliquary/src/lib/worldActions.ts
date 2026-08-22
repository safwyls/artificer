import { downloadURL } from "./api";
import { custodyOf, type WorldStatus } from "./types";

/**
 * The verbs a world offers, and to whom. This is the permission model the
 * old page encoded in `worldActions()`, moved somewhere a test can read it:
 * every rule below is a port of one line there, and the confirm texts are
 * word for word.
 *
 * The server enforces all of it regardless. What this decides is what the
 * UI *offers* — and, for an account without world custody, every mutating
 * affordance is absent rather than disabled: a greyed-out "Check out" is a
 * promise the account will never be able to keep.
 */

export type Placement = "primary" | "quiet" | "overflow" | "hidden";

export interface Confirm {
  title: string;
  body: string;
  confirmLabel: string;
}

export interface WorldAction {
  id: string;
  label: string;
  /** Where this sits on the world card, and on the world's own page. The
   * card keeps one primary and two quiet verbs; its page has room for the
   * rest inline. */
  card: Placement;
  detail: Placement;
  danger?: boolean;
  /** A plain link (a download, or the world's page) rather than a call. */
  href?: string;
  /** Needs a .tar picked first; the page wires the file input. */
  upload?: "checkin" | "import";
  confirm?: Confirm;
  run?: () => void;
}

export interface Actor {
  username: string | null;
  isAdmin: boolean;
  /** The savesync grant, or admin. */
  canSync: boolean;
}

/** What the page must supply: one callback per verb that changes something.
 * They are the page's mutations, so the toasts and refetching live there. */
export interface WorldHandlers {
  checkout: (takeover: boolean) => void;
  renew: (sessionID: number) => void;
  claim: () => void;
  unclaim: () => void;
  requestHandback: (kind: string) => void;
  release: () => void;
  serverGive: () => void;
  serverTake: () => void;
  remove: () => void;
}

export function worldActions(
  status: WorldStatus,
  me: Actor,
  h: WorldHandlers,
): WorldAction[] {
  const w = status.world;
  const holder = status.holder;
  const custody = custodyOf(status);
  const mine = Boolean(holder && holder.username === me.username);
  const out: WorldAction[] = [];

  // --- the one primary, chosen by custody state ---
  if (me.canSync && custody === "free") {
    out.push({ id: "checkout", label: "Check out", card: "primary", detail: "primary", run: () => h.checkout(false) });
  }
  if (me.canSync && mine) {
    out.push({
      id: "checkin",
      label: "Check in…",
      card: "primary",
      detail: "primary",
      upload: "checkin",
    });
  }
  if (me.canSync && holder && !mine && holder.claimable) {
    out.push({
      id: "takeover",
      label: "Take over expired hold",
      card: "primary",
      detail: "primary",
      confirm: {
        title: "Take over the expired hold?",
        body: "The old holder's late check-in will be kept and flagged, not lost.",
        confirmLabel: "Take over",
      },
      run: () => h.checkout(true),
    });
  }

  // --- quiet, on both surfaces ---
  if (me.canSync && mine && holder) {
    out.push({
      id: "renew",
      label: "Renew hold",
      card: "quiet",
      detail: "quiet",
      run: () => h.renew(holder.sessionId),
    });
  }
  if (me.canSync && holder && !mine && !status.claimedBy) {
    out.push({ id: "claim", label: "Claim next", card: "quiet", detail: "quiet", run: h.claim });
  }
  if (me.canSync && status.claimedBy === me.username) {
    out.push({
      id: "unclaim",
      label: "Withdraw claim",
      card: "quiet",
      detail: "quiet",
      run: h.unclaim,
    });
  }
  if (status.head) {
    // A read-only account keeps this: seeing the worlds and downloading a
    // version is exactly what an account without custody may do.
    out.push({
      id: "download-head",
      label: "Download head",
      card: "quiet",
      detail: "quiet",
      href: downloadURL(w.id, status.head.id),
    });
  }
  // History is a page now, not a fold-out — so on that page there is
  // nothing left to link to.
  out.push({ id: "history", label: "History", card: "quiet", detail: "hidden", href: `/worlds/${w.id}` });

  // --- the rare and the admin verbs ---
  if (me.canSync && !holder) {
    out.push({ id: "import", label: "Import…", card: "overflow", detail: "quiet", upload: "import" });
  }
  if (me.isAdmin) {
    if (w.agentUrl && !holder && status.head) {
      out.push({
        id: "server-give",
        label: "Host on dedicated server",
        card: "overflow",
        detail: "quiet",
        run: h.serverGive,
      });
    }
    if (w.agentUrl && holder?.serverHeld) {
      out.push({
        id: "server-take",
        label: "Take back from server",
        card: "overflow",
        detail: "quiet",
        run: h.serverTake,
      });
    }
    if (holder && !holder.serverHeld && !holder.requestedKind) {
      // The holder went quiet: ask their companion to hand the world back
      // on its next poll. Check-in is the useful one — a checkpoint never
      // moves the head, so releasing after one would hand the next player a
      // save from before this session started.
      out.push({
        id: "request-checkin",
        label: "Ask holder to check in",
        card: "overflow",
        detail: "quiet",
        run: () => h.requestHandback("checkin"),
      });
      if (w.checkpoints) {
        out.push({
          id: "request-checkpoint",
          label: "Ask for a checkpoint",
          card: "overflow",
          detail: "quiet",
          run: () => h.requestHandback("checkpoint"),
        });
      }
    }
    if (holder?.requestedKind) {
      out.push({
        id: "request-withdraw",
        label: "Withdraw the request",
        card: "overflow",
        detail: "quiet",
        run: () => h.requestHandback(""),
      });
    }
    if (holder) {
      out.push({
        id: "release",
        label: "Force release",
        card: "overflow",
        detail: "quiet",
        danger: true,
        confirm: {
          title: "Force-release this hold?",
          body: "Anything they have not sent is left on their machine.",
          confirmLabel: "Force release",
        },
        run: h.release,
      });
    }
    // Settings used to unfold under the card; it is a tab on the world's
    // own page now.
    out.push({
      id: "settings",
      label: "Settings",
      card: "overflow",
      detail: "hidden",
      href: `/worlds/${w.id}?tab=settings`,
    });
    out.push({
      id: "delete",
      label: "Delete",
      card: "overflow",
      detail: "overflow",
      danger: true,
      confirm: {
        title: `Delete ${w.name}?`,
        body: "Every stored version of this world goes with it.",
        confirmLabel: "Delete the world",
      },
      run: h.remove,
    });
  }
  return out;
}

export const at = (actions: WorldAction[], surface: "card" | "detail", where: Placement) =>
  actions.filter((a) => a[surface] === where);
