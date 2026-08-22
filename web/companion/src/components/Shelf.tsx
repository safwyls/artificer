import { useState } from "react";
import { GameTile, tileKey } from "./GameTile";
import { ScanTrail } from "./ScanTrail";
import { SectionHeader } from "./Panel";
import { Button } from "./ui/button";
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

/**
 * The installed-game shelf. Hidden entries collapse into one dashed tile
 * that says how many and offers to show them — Steam's own
 * redistributables, runtimes and controller configs start out this way,
 * and a shelf of those is a shelf nobody reads.
 */
export function Shelf({
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
  const games = state.discovered?.games ?? [];
  const links = state.links ?? [];
  const worlds = state.sync?.worlds ?? [];
  const hiddenCount = games.filter((g) => g.hidden).length;
  const shown = games.filter((g) => showHidden || !g.hidden);

  return (
    <section className="flex flex-col gap-2.5">
      <SectionHeader
        title="Installed games"
        hint="linked games in colour — click a greyed tile to link it"
      >
        <Button size="sm" onClick={onRescan}>
          Rescan
        </Button>
        <Button size="sm" onClick={onLinkByHand}>
          Link a folder by hand…
        </Button>
      </SectionHeader>

      <div className="grid grid-cols-[repeat(auto-fill,minmax(130px,1fr))] gap-3.5">
        {shown.map((game) => {
          const link = linkFor(game, links);
          const world = link
            ? worlds.find((w) => w.world.id === link.worldId)?.world.name
            : undefined;
          return (
            // Keyed by the game's identity, never by index: a poll that
            // reorders or filters must not remount a tile and re-fetch
            // its cover.
            <GameTile
              key={tileKey(game)}
              game={game}
              art={art}
              linked={Boolean(link)}
              worldName={world}
              active={activeKey === tileKey(game)}
              onOpen={() => onOpen(game)}
            />
          );
        })}
        {hiddenCount ? (
          <button
            type="button"
            onClick={() => setShowHidden((v) => !v)}
            className="flex flex-col items-center justify-center gap-1.5 rounded-[6px] border border-dashed border-edge p-2.5 text-center text-[12px] text-mist hover:border-gold hover:text-parchment"
          >
            <span>
              {hiddenCount} {hiddenCount === 1 ? "entry" : "entries"} hidden
            </span>
            <span className="text-goldhi">{showHidden ? "hide them again" : "show them"}</span>
          </button>
        ) : null}
        {!shown.length && !hiddenCount ? (
          <p className="col-span-full text-[12px] italic text-mist">
            No games found. The scan trail below says where it looked — if your Steam folder is missing or was
            rejected, set it in the settings. Any save folder can also be linked by hand.
          </p>
        ) : null}
        {!shown.length && hiddenCount ? (
          <p className="col-span-full text-[12px] italic text-mist">Every game found here is hidden.</p>
        ) : null}
      </div>

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
        <p className="text-[12px] italic text-ember">Save-location catalogue unavailable: {hints.error}</p>
      ) : hints.available ? (
        <p className="text-[12px] italic text-mist">
          Save locations known for {hints.known} of these games (Ludusavi manifest, via the sync service).
        </p>
      ) : hints.available === false ? (
        <p className="text-[12px] italic text-mist">
          The sync service has no save-location catalogue loaded — folders are found by search alone.
        </p>
      ) : null}

      <ScanTrail probes={state.discovered?.probes ?? []} />
    </section>
  );
}
