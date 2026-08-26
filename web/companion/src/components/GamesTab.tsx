import { useMemo, useState, type ReactNode } from "react";
import { Search } from "lucide-react";
import { GameTile, tileKey } from "./GameTile";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";
import type { Artwork, CompanionState, DiscoveredGame, Link } from "../lib/types";

/**
 * linkFor finds the link covering a discovered game: by the title
 * recorded when linking, else by a save folder matching one of its
 * candidates. A game linked before app ids were recorded still matches.
 */
export function linkFor(game: DiscoveredGame, links: Link[]): Link | undefined {
  return links.find(
    (l) =>
      (l.gameTitle && l.gameTitle === game.name) ||
      (game.saveDirs ?? []).some((c) => c.path === l.dir),
  );
}

export type GameFilter = "all" | "linked" | "unlinked";

/** Case- and space-insensitive: nobody types a game's name exactly. */
export const matches = (game: DiscoveredGame, q: string) =>
  !q.trim() || game.name.toLowerCase().includes(q.trim().toLowerCase());

function Segmented({
  value,
  onChange,
  counts,
}: {
  value: GameFilter;
  onChange: (v: GameFilter) => void;
  counts: Record<GameFilter, number>;
}) {
  const options: { id: GameFilter; label: string }[] = [
    { id: "all", label: "All" },
    { id: "linked", label: "Linked" },
    { id: "unlinked", label: "Unlinked" },
  ];
  return (
    <div className="flex gap-0.5 rounded border border-edge p-0.5">
      {options.map(({ id, label }) => (
        <button
          key={id}
          type="button"
          aria-pressed={value === id}
          onClick={() => onChange(id)}
          className={cn(
            "rounded-[3px] px-3 py-1 text-[12.5px] transition-colors",
            value === id ? "bg-panel text-goldhi" : "text-mist hover:text-parchment",
          )}
        >
          {label} {counts[id]}
        </button>
      ))}
    </div>
  );
}

function Section({ label, count, sub, gold, children }: {
  label: string;
  count: number;
  sub?: ReactNode;
  gold?: boolean;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex flex-wrap items-baseline gap-3">
        <h2
          className={cn(
            "text-[11px] font-semibold uppercase tracking-[0.12em]",
            gold ? "text-gold" : "text-mist",
          )}
        >
          {label} · {count}
        </h2>
        {sub ? <span className="text-[12.5px] text-mist">{sub}</span> : null}
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-3.5">{children}</div>
    </section>
  );
}

/**
 * The games library: setup, visited rarely, and its own tab.
 *
 * The old grid put the one linked tile among twelve dimmed ones with no
 * search and no way to narrow it. Linked games have their own section
 * first, and the toolbar carries a search field and three filters — the
 * two questions anyone actually arrives with are "where is that game"
 * and "which of these are linked".
 *
 * Hidden entries are a line of text below the grid, not a fake tile
 * inside it: Steam's own redistributables, runtimes and controller
 * configs start out hidden, and a control shaped like a game is a
 * control that gets clicked by accident.
 */
export function GamesTab({
  state,
  art,
  artEmpty,
  artError,
  hints,
  activeKey,
  onOpen,
  onRescan,
  onLinkByHand,
}: {
  state: CompanionState;
  art: Record<string, Artwork>;
  artEmpty: boolean;
  artError?: string;
  hints: { available?: boolean; known?: number; error?: string };
  activeKey: string | null;
  onOpen: (game: DiscoveredGame) => void;
  onRescan: () => void;
  onLinkByHand: () => void;
}) {
  const [showHidden, setShowHidden] = useState(false);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<GameFilter>("all");

  const games = state.discovered?.games ?? [];
  const links = state.links ?? [];
  const worlds = state.sync?.worlds ?? [];
  const hiddenCount = games.filter((g) => g.hidden).length;

  const { linked, unlinked, counts } = useMemo(() => {
    const visible = games.filter((g) => (showHidden || !g.hidden) && matches(g, query));
    const linked = visible.filter((g) => linkFor(g, links));
    const unlinked = visible.filter((g) => !linkFor(g, links));
    return {
      linked,
      unlinked,
      counts: { all: visible.length, linked: linked.length, unlinked: unlinked.length },
    };
  }, [games, links, query, showHidden]);

  const knowsFolder = unlinked.filter((g) => (g.saveDirs ?? []).length).length;

  const tile = (game: DiscoveredGame) => {
    const link = linkFor(game, links);
    return (
      // Keyed by the game's identity, never by index: a poll that
      // reorders or filters must not remount a tile and re-fetch its
      // cover.
      <GameTile
        key={tileKey(game)}
        game={game}
        art={art}
        linked={Boolean(link)}
        worldName={link ? worlds.find((w) => w.world.id === link.worldId)?.world.name : undefined}
        active={activeKey === tileKey(game)}
        onOpen={() => onOpen(game)}
      />
    );
  };

  return (
    <div className="flex flex-col gap-[18px] px-7 pb-[26px] pt-[22px]">
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-[340px] flex-1">
          <Search className="pointer-events-none absolute left-[11px] top-[11px] h-3.5 w-3.5 text-mist" aria-hidden />
          <input
            type="search"
            aria-label="Search installed games"
            placeholder="Search installed games"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full rounded border border-edge bg-ink py-2 pl-8 pr-2.5 text-[13.5px] text-parchment placeholder:text-mist/60"
          />
        </div>
        <Segmented value={filter} onChange={setFilter} counts={counts} />
        <div className="ml-auto flex gap-2">
          <Button onClick={onRescan}>Rescan</Button>
          <Button onClick={onLinkByHand}>Link a folder by hand…</Button>
        </div>
      </div>

      {/* Linked first: in the old grid the one linked tile was lost among
          twelve dimmed ones. */}
      {filter !== "unlinked" && linked.length ? (
        <Section label="Linked" count={linked.length} gold>
          {linked.map(tile)}
        </Section>
      ) : null}

      {filter !== "linked" && unlinked.length ? (
        <Section
          label="Not linked"
          count={unlinked.length}
          sub={
            knowsFolder
              ? `A save folder is already known for ${knowsFolder} of these.`
              : undefined
          }
        >
          {unlinked.map(tile)}
        </Section>
      ) : null}

      {!counts.all ? (
        <p className="text-[12.5px] italic text-mist">
          {query.trim()
            ? `Nothing installed here matches “${query.trim()}”.`
            : hiddenCount && !showHidden
              ? "Every game found here is hidden."
              : "No games found. Diagnostics, at the bottom of the window, has the scan trail — if your Steam folder is missing or was rejected, set it in Settings. Any save folder can also be linked by hand."}
        </p>
      ) : null}

      {hiddenCount ? (
        <p className="text-[12.5px] text-mist">
          {hiddenCount} {hiddenCount === 1 ? "entry was" : "entries were"} hidden as duplicates or
          launchers.{" "}
          <button
            type="button"
            onClick={() => setShowHidden((v) => !v)}
            className="rounded-[3px] text-goldhi hover:text-gold hover:underline"
          >
            {showHidden ? "Hide them again" : "Show them"}
          </button>
        </p>
      ) : null}

      {/* Covers and save locations both degrade to nothing rather than to
          an error — but when the *service* is the reason, saying so points
          at the panel that can fix it. */}
      {artError ? (
        <p className="text-[12px] italic text-ember">Cover art unavailable: {artError}</p>
      ) : artEmpty ? (
        <p className="text-[12px] italic text-mist">
          The sync service has no cover art for these games — check its Cover art panel.
        </p>
      ) : null}
      {hints.error ? (
        <p className="text-[12px] italic text-ember">
          Save-location catalogue unavailable: {hints.error}
        </p>
      ) : hints.available === false ? (
        <p className="text-[12px] italic text-mist">
          The sync service has no save-location catalogue loaded — folders are found by search alone.
        </p>
      ) : null}
    </div>
  );
}
