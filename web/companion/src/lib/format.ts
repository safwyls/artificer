/** Timestamps in the player's own locale and zone. A hold expiring is a
 * question about *their* clock. */
export function fmtTime(t: string | undefined): string {
  if (!t) return "";
  const d = new Date(t);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString();
}

/**
 * The age of what is on screen. Custody is shared state — someone else
 * checking a world in is the whole reason this page exists — so "when did
 * we last hear from the service" is worth saying rather than leaving
 * people to guess.
 */
export function freshness(polledAt: string | undefined, now = Date.now()): string {
  if (!polledAt) return "not synced yet";
  const at = new Date(polledAt).getTime();
  if (Number.isNaN(at)) return "not synced yet";
  const secs = Math.max(0, Math.round((now - at) / 1000));
  if (secs < 10) return "up to date";
  if (secs < 90) return `synced ${secs}s ago`;
  return `synced ${Math.round(secs / 60)} min ago`;
}

/** A clock time alone — "18:02" — for a stamp sitting beside a date that
 * is already implied by "today". */
export function fmtClock(t: string | undefined): string {
  if (!t) return "";
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/**
 * When something happened, at the coarseness a player actually reads:
 * minutes for the last hour, the clock for today, the weekday for this
 * week, the date beyond that. The mono head-meta line carries this.
 */
export function fmtWhen(t: string | undefined, now = Date.now()): string {
  if (!t) return "";
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return "";
  const secs = Math.round((now - d.getTime()) / 1000);
  if (secs < 0) return fmtClock(t);
  if (secs < 60) return "just now";
  if (secs < 3600) return `${Math.round(secs / 60)} min ago`;
  if (secs < 86_400) return fmtClock(t);
  if (secs < 7 * 86_400) return d.toLocaleDateString([], { weekday: "long" });
  return d.toLocaleDateString();
}

/**
 * A span, said the way a hold is talked about: "47h left", "2h 12m",
 * "8m". Hours and minutes only — a hold lasts 48 hours, so days would
 * round away the whole scale and seconds would be a stopwatch.
 */
export function fmtSpan(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  const mins = Math.floor(ms / 60_000);
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  if (h >= 10) return `${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${Math.max(1, m)}m`;
}

/** Bytes as a save is talked about — "240 MB", "1.8 GB". Decimal units,
 * because that is what the folder's own properties dialog says. */
export function fmtBytes(n: number | undefined): string {
  if (!n || n < 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

/** "2 libraries", "1 library" — the scan trail counts both halves. */
export const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
