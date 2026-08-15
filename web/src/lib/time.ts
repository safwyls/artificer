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

/** The two stamps a save player carries, either of which can date them.
 *
 * lastSeen is flamekeeper's own observation of the player leaving. lastOnline is
 * the save's LastOnlineDateTime, which Palworld writes at *login* and never
 * updates — so on its own it reports when someone arrived, understating an
 * offline player by the whole length of their last session. It stays as the
 * fallback for servers flamekeeper has no history for, but every label below says
 * "joined" when it's in use: a join time presented as a last-seen time is the
 * bug these two functions exist to end. */
export interface SeenStamps {
  lastSeen: number;
  lastOnline: number;
}

/** Bare form, for a list whose own heading supplies the "last seen": "2h ago",
 * "joined 5h ago", or "" when neither stamp exists. */
export function seenLabel(p: SeenStamps): string {
  if (p.lastSeen) return lastSeenLabel(p.lastSeen);
  if (p.lastOnline) return `joined ${lastSeenLabel(p.lastOnline)}`;
  return "";
}

/** Inline form for a detail line that has to say what the time means on its
 * own: "seen 2h ago", "joined 5h ago", or "". */
export function seenPhrase(p: SeenStamps): string {
  if (p.lastSeen) return `seen ${lastSeenLabel(p.lastSeen)}`;
  if (p.lastOnline) return `joined ${lastSeenLabel(p.lastOnline)}`;
  return "";
}

/** Standalone form for a map marker's sublabel, where the phrase carries
 * itself: "Last seen 2h ago", "Joined 5h ago", or "Offline". */
export function seenSentence(p: SeenStamps): string {
  if (p.lastSeen) return `Last seen ${lastSeenLabel(p.lastSeen)}`;
  if (p.lastOnline) return `Joined ${lastSeenLabel(p.lastOnline)}`;
  return "Offline";
}
