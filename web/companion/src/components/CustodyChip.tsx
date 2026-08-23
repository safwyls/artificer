import { Clock, Lock, LockOpen } from "lucide-react";
import { fmtSpan } from "../lib/format";
import { cn } from "../lib/utils";
import { holdLeft, holdIsPressing, type Custody, type Link } from "../lib/types";

/**
 * One world's custody state in a word, drawn from the single `Custody`
 * value the row's primary action is also drawn from. The chip and the
 * button cannot disagree because neither decides anything: they read.
 *
 * Gold means *you* hold it. Someone else's hold is grey, not gold —
 * that one difference is what makes the world you hold findable in a
 * list without reading a single label.
 */
export function CustodyChip({ custody, className }: { custody: Custody; className?: string }) {
  const shape = "inline-flex items-center gap-1.5 rounded-full border px-[11px] py-0.5 text-[12px]";
  const icon = "h-[11px] w-[11px]";
  switch (custody.state) {
    case "free":
      return (
        <span className={cn(shape, "border-ok bg-[#14200f] text-ok", className)}>
          <LockOpen className={icon} aria-hidden />
          Free
        </span>
      );
    case "expired":
      return (
        <span className={cn(shape, "border-ember bg-[#26130e] text-ember", className)}>
          <Clock className={icon} aria-hidden />
          Hold expired
        </span>
      );
    case "gone":
      return (
        <span className={cn(shape, "border-ember bg-[#26130e] text-ember", className)}>
          Not on the service
        </span>
      );
    case "held":
      // Grey, deliberately: the gold chip is reserved for the one world
      // this machine holds.
      return (
        <span className={cn(shape, "border-mist bg-well text-mist", className)}>
          <Lock className={icon} aria-hidden />
          Held
        </span>
      );
    default:
      return (
        <span className={cn(shape, "border-gold bg-[#23180c] text-goldhi", className)}>
          <Lock className={icon} aria-hidden />
          {custody.state === "mine" ? "Yours" : "Yours — fetching"}
        </span>
      );
  }
}

/**
 * The countdown, shown only when a hold is actually pressure — under
 * three hours left. Above that the row just names who has it: a 47-hour
 * timer ticking on screen is noise pretending to be urgency.
 */
export function HoldCountdown({ custody, now = Date.now() }: { custody: Custody; now?: number }) {
  if (!holdIsPressing(custody, now)) return null;
  return (
    <span className="inline-flex items-center gap-1.5 rounded-[3px] border border-ember/45 px-[7px] py-px font-mono text-[11px] text-ember">
      <Clock className="h-2.5 w-2.5" aria-hidden />
      {`${fmtSpan(holdLeft(custody, now))} left`}
    </span>
  );
}

/**
 * The sentence beside the chip: what the state means for the player
 * standing in front of it, and nothing the chip already said.
 *
 * "nobody holds this world" is gone — it restated the Free chip from the
 * other side of the row. A free world with nothing queued behind it says
 * nothing at all, and the grouping carries the meaning.
 */
export function custodyLine(custody: Custody, link: Link, me: string | undefined, now = Date.now()): string {
  const next = custody.claimedBy
    ? custody.claimedBy === me
      ? "you're next"
      : `next in line: ${custody.claimedBy}`
    : "";
  const left = fmtSpan(holdLeft(custody, now));
  switch (custody.state) {
    case "gone":
      return `world #${link.worldId} is not on the service any more`;
    case "free":
      return next;
    case "mine":
      return [left ? `${left} left on the hold` : "the save is on this machine", next]
        .filter(Boolean)
        .join(" · ");
    case "fetching":
      return "fetching it to this machine…";
    case "expired":
      return [`held by ${custody.holder} — the hold expired`, next].filter(Boolean).join(" · ");
    default:
      return [`held by ${custody.holder}`, next].filter(Boolean).join(" · ");
  }
}
