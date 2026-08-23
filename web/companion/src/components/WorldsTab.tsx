import { useState, type ReactNode } from "react";
import { custodyOf, type Artwork, type CompanionState, type CustodyState, type Link, type SyncWorld } from "../lib/types";
import { cn } from "../lib/utils";
import { OfflineBanner, QueuedToSend } from "./Offline";
import { WorldRow } from "./WorldRow";

/**
 * A group of worlds, and the whole reason the Worlds tab is readable:
 * grouping by what you can do with a world replaces the status prose
 * that used to sit on every row.
 *
 * The group you hold is bordered in gold, so the one world locked to this
 * machine is findable at a glance in a list of any length.
 */
export function WorldGroup({
  label,
  sub,
  count,
  gold,
  children,
}: {
  label: string;
  sub?: string;
  count?: number;
  gold?: boolean;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline gap-3">
        <h2
          className={cn(
            "font-mono text-[10px] uppercase tracking-[0.12em]",
            gold ? "text-gold" : "text-mist",
          )}
        >
          {label}
          {count !== undefined ? ` · ${count}` : ""}
        </h2>
        {sub ? <span className="text-[12.5px] text-mist">{sub}</span> : null}
      </div>
      <div
        className={cn(
          // Not `overflow-hidden`, however much the rounded corners want
          // it: a row's overflow menu is positioned inside this card, and
          // clipping the card clipped the menu to the row it opened from.
          // The corners are kept by rounding the first and last rows
          // instead, which is what the clip was actually for — a row's
          // hover fill squaring off the card's corner.
          "rounded-panel border bg-panel [&>*+*]:border-t [&>*+*]:border-edge",
          "[&>*:first-child]:rounded-t-panel [&>*:last-child]:rounded-b-panel",
          gold ? "border-gold/45" : "border-edge",
        )}
      >
        {children}
      </div>
    </section>
  );
}

/** Which group a world belongs in. Derived from custody and stored
 * nowhere — the same rule the chip and the primary action read. */
export function groupOf(state: CustodyState): "yours" | "free" | "held" {
  if (state === "mine" || state === "fetching") return "yours";
  if (state === "free") return "free";
  return "held";
}

/**
 * The Worlds tab: everything this machine has linked, in three groups —
 * what you hold, what you can take, and what someone else has.
 */
export function WorldsTab({
  state,
  art,
  offline,
  retrying,
  onRetry,
  onOpenGames,
}: {
  state: CompanionState;
  art: Record<string, Artwork>;
  offline?: boolean;
  retrying?: boolean;
  onRetry?: () => void;
  onOpenGames: () => void;
}) {
  // Offline, the worlds nobody here holds are hidden by default: custody
  // cannot be confirmed, so checking one out could collide with someone
  // else. They are one click away, read-only, for anyone who just wants
  // to look.
  const [showOthers, setShowOthers] = useState(false);
  const links = state.links ?? [];
  const worlds = state.sync?.worlds ?? [];
  const me = state.sync?.username;
  const launchOnCheckout = state.config?.launchOnCheckout ?? true;
  const worldFor = (l: Link): SyncWorld | undefined => worlds.find((w) => w.world.id === l.worldId);

  const grouped: Record<"yours" | "free" | "held", Link[]> = { yours: [], free: [], held: [] };
  for (const link of links) {
    grouped[groupOf(custodyOf(link, worldFor(link), me, true).state)].push(link);
  }

  const row = (link: Link) => (
    <WorldRow
      key={link.worldId}
      link={link}
      world={worldFor(link)}
      me={me}
      art={art}
      configured
      offline={offline}
      launchOnCheckout={launchOnCheckout}
    />
  );

  const games = (state.discovered?.games ?? []).filter((g) => !g.hidden);
  const linkedGames = new Set(links.map((l) => l.gameTitle).filter(Boolean));

  const others = grouped.free.length + grouped.held.length;
  const hideOthers = Boolean(offline) && !showOthers && others > 0;
  const queue = state.sync?.queue ?? [];

  return (
    <div className="flex flex-col gap-[22px] px-7 pb-[26px] pt-[22px]">
      {offline ? (
        <OfflineBanner
          error={state.sync?.lastError}
          retrying={Boolean(retrying)}
          onRetry={() => onRetry?.()}
        />
      ) : null}

      {grouped.yours.length ? (
        <WorldGroup
          label="Checked out to you"
          sub={
            offline
              ? "playable offline — the hold stands until the vault answers"
              : "locked to this machine until you check it in"
          }
          gold
        >
          {grouped.yours.map(row)}
        </WorldGroup>
      ) : null}

      {offline ? <QueuedToSend queue={queue} /> : null}

      {grouped.free.length && !hideOthers ? (
        <WorldGroup label="Free to take" count={grouped.free.length}>
          {grouped.free.map(row)}
        </WorldGroup>
      ) : null}

      {grouped.held.length && !hideOthers ? (
        <WorldGroup label="Held by someone else" count={grouped.held.length}>
          {grouped.held.map(row)}
        </WorldGroup>
      ) : null}

      {hideOthers ? (
        <div className="rounded-panel border border-dashed border-edge bg-well px-[18px] py-3.5 text-[12.5px] text-mist">
          The other {others} world{others === 1 ? " is" : "s are"} hidden while offline — custody
          can&apos;t be confirmed, so checking one out could collide with someone else.{" "}
          <button
            type="button"
            onClick={() => setShowOthers(true)}
            className="rounded-[3px] text-goldhi hover:text-gold hover:underline"
          >
            Show them read-only
          </button>
        </div>
      ) : null}

      {links.length ? null : (
        <p className="text-[13px] italic text-mist">
          Nothing linked yet — link an installed game from the Games tab, or ask whoever runs your
          sync service which world to join.
        </p>
      )}

      {/* The only pointer to the Games tab from Worlds. */}
      <div className="flex flex-wrap items-center gap-2.5 rounded-panel border border-dashed border-edge bg-well px-[18px] py-3 text-[12.5px] text-mist">
        <span>
          {games.length} game{games.length === 1 ? "" : "s"} installed on this machine,{" "}
          {linkedGames.size} linked to a world.
        </span>
        <button type="button" onClick={onOpenGames} className="rounded-[3px] text-goldhi hover:text-gold hover:underline">
          Open the games library
        </button>
      </div>
    </div>
  );
}
