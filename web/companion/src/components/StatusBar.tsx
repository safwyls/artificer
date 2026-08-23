import type { CompanionState } from "../lib/types";

/**
 * The status bar: what this machine holds, in one line, and the way into
 * diagnostics. Versions and build hashes are not here — they are the
 * first thing a bug report needs and the last thing a player does, so
 * they sit in Settings › Diagnostics with everything else of that kind.
 *
 * This used to live in HeaderBar.tsx beside a header bar that no longer
 * exists: the app's identity and its sync report are the titlebar's now,
 * and "Sync now" sits beside the tabs.
 */
export function StatusBar({
  state,
  onDiagnostics,
}: {
  state: CompanionState | undefined;
  onDiagnostics: () => void;
}) {
  const links = state?.links ?? [];
  const worlds = state?.sync?.worlds ?? [];
  const games = (state?.discovered?.games ?? []).filter((g) => !g.hidden);
  const n = (count: number, one: string, many: string) =>
    `${count} ${count === 1 ? one : many}`;
  const left = state?.sync?.configured
    ? `${n(worlds.length, "world", "worlds")} · ${n(games.length, "game", "games")} installed, ${
        links.length
      } linked`
    : `${n(games.length, "game", "games")} found · nothing linked yet`;

  return (
    <footer className="flex flex-none items-center gap-4 border-t border-edge bg-well px-7 py-2.5 text-[12px] text-mist">
      <span>{left}</span>
      <button
        type="button"
        onClick={onDiagnostics}
        className="ml-auto rounded-[3px] text-[12px] text-goldhi hover:text-gold hover:underline"
      >
        Diagnostics
      </button>
    </footer>
  );
}
