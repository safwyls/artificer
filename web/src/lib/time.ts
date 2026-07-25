/** Compact relative-time labels, shared by the pals/guilds/map views. */

/** Seconds-ago → "42s ago" / "5m ago" / "3h ago" / "2d ago". */
export function agoSeconds(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

/** ISO timestamp → "…ago" label (save written / parsed footers). */
export function agoLabel(iso: string): string {
  return agoSeconds((Date.now() - new Date(iso).getTime()) / 1000);
}

/** Unix-seconds last-seen → label. "" when the save recorded none;
 * "just now" under 90s, since offline times are only save-accurate. */
export function lastSeenLabel(unixSeconds: number): string {
  if (!unixSeconds) return "";
  const s = Date.now() / 1000 - unixSeconds;
  if (s < 90) return "just now";
  return agoSeconds(s);
}
