import { Clock, Lock, LockOpen } from "lucide-react";
import { custodyOf, type WorldStatus } from "../lib/types";
import { fmtTime } from "../lib/format";
import { cn } from "../lib/utils";

/**
 * One world's custody state, in a word: Free (green), Held (gold), Hold
 * expired (ember). The chip is what drives the card's single primary
 * action, so the two can never disagree — both read custodyOf().
 */
export function CustodyChip({ status, className }: { status: WorldStatus; className?: string }) {
  const custody = custodyOf(status);
  const shape =
    "inline-flex items-center gap-1.5 rounded-full border px-3 py-0.5 text-[12px]";
  if (custody === "free") {
    return (
      <span className={cn(shape, "border-ok bg-[#14200f] text-ok", className)}>
        <LockOpen className="h-3 w-3" aria-hidden />
        Free
      </span>
    );
  }
  if (custody === "expired") {
    return (
      <span className={cn(shape, "border-ember bg-[#26130e] text-ember", className)}>
        <Clock className="h-3 w-3" aria-hidden />
        Hold expired
      </span>
    );
  }
  return (
    <span className={cn(shape, "border-gold bg-[#23180c] text-goldhi", className)}>
      <Lock className="h-3 w-3" aria-hidden />
      Held
    </span>
  );
}

/**
 * The sentence beside the chip: who holds it, until when, who is next, and
 * — when someone has asked the holder for the world back — how long that
 * has gone unanswered. A request nobody can see the state of is a request
 * nobody trusts.
 */
export function holderLine(status: WorldStatus, myUsername: string | null): string {
  const h = status.holder;
  const next = status.claimedBy ? ` · next claim: ${status.claimedBy}` : "";
  if (!h) return `nobody holds this world${next}`;
  const who = h.username === myUsername ? "you" : h.username;
  const where = h.serverHeld ? " (on the dedicated server)" : "";
  const held = h.claimable
    ? `held by ${who}${where} — claimable`
    : `held by ${who}${where} until ${fmtTime(h.expiresAt)}`;
  return held + next;
}

/** What has been asked of the holder, and when. Empty when nothing has. */
export function requestLine(status: WorldStatus): string {
  const h = status.holder;
  if (!h?.requestedKind) return "";
  const what = h.requestedKind === "checkin" ? "check in and release" : "push a checkpoint";
  const since = h.requestedAt ? ` · asked ${fmtTime(h.requestedAt)}` : "";
  return `waiting for ${h.username}'s companion to ${what}${since} — it answers within a minute of being online; if their machine is asleep the request stands until it wakes.`;
}
