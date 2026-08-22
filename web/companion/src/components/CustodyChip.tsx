import { Clock, Lock, LockOpen } from "lucide-react";
import { fmtTime } from "../lib/format";
import { cn } from "../lib/utils";
import type { Custody, Link, SyncWorld } from "../lib/types";

/** One world's custody state in a word, in the vault's own colours: Free
 * green, held gold, an expired hold ember. */
export function CustodyChip({ custody, className }: { custody: Custody; className?: string }) {
  const shape = "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px]";
  if (custody === "free") {
    return (
      <span className={cn(shape, "border-ok bg-[#14200f] text-ok", className)}>
        <LockOpen className="h-2.5 w-2.5" aria-hidden />
        Free
      </span>
    );
  }
  if (custody === "expired") {
    return (
      <span className={cn(shape, "border-ember bg-[#26130e] text-ember", className)}>
        <Clock className="h-2.5 w-2.5" aria-hidden />
        Hold expired
      </span>
    );
  }
  if (custody === "gone") {
    return (
      <span className={cn(shape, "border-ember bg-[#26130e] text-ember", className)}>
        Not on the service
      </span>
    );
  }
  return (
    <span className={cn(shape, "border-gold bg-[#23180c] text-goldhi", className)}>
      <Lock className="h-2.5 w-2.5" aria-hidden />
      {custody === "mine" ? "You hold this world" : custody === "fetching" ? "Yours — fetching" : "Held"}
    </span>
  );
}

/** The sentence under the world's name: what the state means for the
 * player standing in front of it. */
export function custodyLine(
  custody: Custody,
  link: Link,
  world: SyncWorld | undefined,
  me: string | undefined,
): string {
  const h = world?.holder;
  const next = world?.claimedBy
    ? world.claimedBy === me
      ? " · you're next"
      : ` · next claim: ${world.claimedBy}`
    : "";
  switch (custody) {
    case "gone":
      return `world #${link.worldId} is not on the service any more`;
    case "free":
      return `nobody holds this world${next}`;
    case "mine":
      return `until ${fmtTime(h?.expiresAt)} · save is on this machine`;
    case "fetching":
      return "fetching it to this machine…";
    case "expired":
      return `held by ${h?.username} — the hold expired${next}`;
    default:
      return `held by ${h?.username} until ${fmtTime(h?.expiresAt)}${next}`;
  }
}
