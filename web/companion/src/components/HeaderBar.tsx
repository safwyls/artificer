import { useEffect, useState } from "react";
import { RefreshCw, Settings } from "lucide-react";
import { freshness } from "../lib/format";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import type { CompanionState } from "../lib/types";

/** The Reliquary mark: the vault's diamond, drawn once here and reused
 * by the empty state at a larger size. */
export function VaultMark({ className, strokeWidth = 1.4 }: { className?: string; strokeWidth?: number }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} className={className} aria-hidden>
      <path d="M12 2l4 4-4 4-4-4z" />
      <path d="M4 12l8 10 8-10-8-4z" />
    </svg>
  );
}

/**
 * Sync state, said in exactly one place: a dot and a relative time.
 *
 * It used to be said three times — here, again in a line at the bottom of
 * the page, and a third time in the scan trail's summary. Three readings
 * of one fact drift, and the player has no way to tell which is current.
 * The scan trail, the tried paths and the build hashes are diagnostics
 * now and live in Settings.
 */
export function HeaderBar({
  state,
  syncing,
  onSyncNow,
  onOpenSettings,
}: {
  state: CompanionState | undefined;
  syncing: boolean;
  onSyncNow: () => void;
  onOpenSettings: () => void;
}) {
  const sync = state?.sync;
  const configured = Boolean(sync?.configured);
  const offline = Boolean(sync?.lastError);

  // Freshness is an age, so it has to be recomputed on a clock of its own
  // — the poll it describes may answer with an unchanged timestamp.
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <header className="flex flex-wrap items-end justify-between gap-6 px-7 pt-5">
      <div>
        <div className="text-[22px] tracking-[0.05em] text-gold">Artificer Companion</div>
        <div className="mt-0.5 text-[12.5px] text-mist">
          {configured
            ? `this machine, syncing as ${sync?.username ?? "…"}`
            : "this machine — not connected to a vault yet"}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3.5 pb-1">
        {configured ? (
          <span className="flex items-center gap-[7px] font-mono text-[11px] text-mist">
            {syncing || sync?.busy ? (
              <RefreshCw className="h-[9px] w-[9px] animate-spin text-gold" aria-hidden />
            ) : (
              <span
                className={cn("h-[7px] w-[7px] rounded-full", offline ? "bg-ember" : "bg-ok")}
                aria-hidden
              />
            )}
            <span>
              {syncing
                ? "syncing…"
                : sync?.busy
                  ? "transfer in progress…"
                  : offline
                    ? "the vault is unreachable"
                    : freshness(sync?.polledAt)}
            </span>
          </span>
        ) : null}
        {configured ? (
          <Button onClick={onSyncNow} disabled={syncing}>
            <RefreshCw className={cn("h-3.5 w-3.5", syncing && "animate-spin")} aria-hidden />
            {syncing ? "Syncing…" : "Sync now"}
          </Button>
        ) : null}
        <Button size="icon" onClick={onOpenSettings} aria-label="Settings">
          <Settings className="h-3.5 w-3.5" aria-hidden />
        </Button>
      </div>
    </header>
  );
}

/**
 * The status bar: what this machine holds, in one line, and the way into
 * diagnostics. Versions and build hashes left the footer — they are the
 * first thing a bug report needs and the last thing a player does, so
 * they sit in Settings › Diagnostics with everything else of that kind.
 */
export function FooterBar({
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
    <footer className="flex items-center gap-4 border-t border-edge bg-well px-7 py-2.5 text-[12px] text-mist">
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
