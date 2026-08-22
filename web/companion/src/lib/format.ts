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

/** "2 libraries", "1 library" — the scan trail counts both halves. */
export const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
