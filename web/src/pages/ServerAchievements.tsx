import { type ReactNode, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { api, ApiError, type AchievementsPlayer } from "../lib/api";
import {
  ASTRALYM,
  BOSS_CHAIN,
  BOUNTY_ROSTER,
  PALPAGOS_TOWERS,
  PANTHALUS,
  RAID_ROSTER,
  WORLD_TREE_RUN,
  arenaRank,
  bossLabel,
  bossesCleared,
  extraTowerKeys,
  isLaboratory,
  mainQuests,
  questLabel,
  raidPalId,
  recordName,
  raidFight,
  splitFieldBosses,
  towerClears,
  towerFight,
  type Boss,
} from "../lib/achievements";
import { palIconUrl } from "../lib/paldex";
import { playerColor } from "../lib/palette";
import { seenSentence } from "../lib/time";
import { cn } from "../lib/utils";
import { SavePathSetup } from "../components/SavePathSetup";
import { SaveReadProgress } from "../components/SaveReadProgress";
import { SaveUpdatingBanner } from "../components/SaveUpdatingBanner";
import { BossFightDialog } from "../components/BossFightDialog";
import { ServerUnreachable } from "../components/ServerUnreachable";

// ---------------------------------------------------------------------------
// This page answers "what's left", not "look how much you did".
//
// A completion percentage would be the obvious hero and the wrong one: on an
// established server the towers are all but done, so the number sits at 95%
// and never moves. The gaps are where the information is — the one tower half
// the guild hasn't touched, the raid boss nobody has summoned — so the towers
// lead, drawn as the run they are, and everything else is a quiet count.
//
// Every figure here is per player. The save has no world-level "this boss is
// dead", only "this player has beaten it", so "the server has cleared X" can
// only ever mean "everyone on it has".
// ---------------------------------------------------------------------------

/** Who has beaten a thing, in the page's one shape: a dot per player, in the
 * colour that player wears everywhere else in palcon. Filled means cleared. */
function ClearDots({
  players,
  cleared,
  what,
}: {
  players: AchievementsPlayer[];
  cleared: (p: AchievementsPlayer) => boolean;
  what: string;
}) {
  const done = players.filter(cleared);
  return (
    <div
      className="flex flex-wrap items-center justify-center gap-1"
      // One label for the row rather than one per dot: a screen reader wants
      // "3 of 4 players", not four separate name-and-state announcements.
      role="img"
      aria-label={
        done.length === 0
          ? `Nobody has beaten ${what}`
          : `${done.length} of ${players.length} have beaten ${what}: ${done.map((p) => p.nickname).join(", ")}`
      }
    >
      {players.map((p) => {
        const hit = cleared(p);
        return (
          <span
            key={p.uid}
            title={`${p.nickname} — ${hit ? "cleared" : "not yet"}`}
            // border-current, not ring-1: an uncoloured `ring` falls back to
            // Tailwind's default ring blue, which is a colour this app doesn't
            // own and read as a third state rather than as "not yet".
            className={cn("h-2 w-2 rounded-full", !hit && "border border-current")}
            style={hit ? { backgroundColor: playerColor(p.uid) } : undefined}
          />
        );
      })}
    </div>
  );
}

/** How a boss battle stands with the group: everyone, some, or nobody. */
type ClearState = "all" | "some" | "none";

/** One boss's portrait, at the size its tier calls for.
 *
 * Uncleared draws sepia and cleared draws in colour — the same reveal the
 * Paldex's wanted posters use, so the two views spend their one flourish on
 * the same idea rather than inventing a second. The ring carries the group's
 * state: brand red once everyone has it, amber while some have.
 */
function BossPortrait({ boss, state, size }: { boss: Boss; state: ClearState; size: "sm" | "md" | "lg" }) {
  const box = size === "lg" ? "h-16 w-16" : size === "md" ? "h-14 w-14" : "h-11 w-11";
  // The Laboratory borrows a pal outline and throws away everything else:
  // flattened to black under a red rim, the way the game presents its
  // modified pals. brightness(0) keeps the alpha and drops every pixel to
  // black; the four offset shadows draw the rim around whatever shape is left.
  // Kept off the sepia/colour track on purpose — it has no "true" appearance
  // to develop into.
  const lab = isLaboratory(boss);
  // The ring and the ground live on a wrapper rather than on the image,
  // because a CSS filter applies to an element's own background too: with
  // both on one <img>, brightness(0) blackened the ground along with the pal
  // and the silhouette became a plain disc. The scale is the second half of
  // the same problem — these icons are tight crops, so the shape needs room
  // inside the circle for the rim to trace against.
  //
  // Uncleared, the Laboratory dims like everything else rather than glowing:
  // the red rim is what makes it look live, so an unbeaten one loses the glow
  // and takes an ash-grey rim instead. Same "not yet" reading as the sepia
  // portraits, in the only vocabulary a silhouette has.
  const cold = state === "none";
  return (
    <span
      className={cn(
        "relative flex shrink-0 items-center justify-center overflow-hidden rounded-full border-2 transition-colors duration-300 motion-reduce:transition-none",
        box,
        state === "all" && "border-brand-red",
        state === "some" && "border-brand-amber",
        state === "none" && "border-paper/15",
        lab ? (cold ? "bg-ink-light" : "bg-[#1b0f0c]") : "bg-ink-light",
      )}
    >
      <img
        src={palIconUrl(boss.palId ?? "")}
        alt=""
        loading="lazy"
        className={cn(
          "h-full w-full object-contain transition-[filter] duration-300 motion-reduce:transition-none",
          !lab && cold && "[filter:grayscale(1)_sepia(0.55)_brightness(0.7)_contrast(1.1)]",
          lab && "scale-[0.72]",
          lab &&
            !cold &&
            "[filter:brightness(0)_drop-shadow(1px_0_0_#E8491D)_drop-shadow(-1px_0_0_#E8491D)_drop-shadow(0_1px_0_#E8491D)_drop-shadow(0_-1px_0_#E8491D)_drop-shadow(0_0_5px_rgba(232,73,29,0.9))]",
          lab &&
            cold &&
            "opacity-60 [filter:brightness(0)_drop-shadow(1px_0_0_#6E635B)_drop-shadow(-1px_0_0_#6E635B)_drop-shadow(0_1px_0_#6E635B)_drop-shadow(0_-1px_0_#6E635B)]",
        )}
      />
    </span>
  );
}

/** A boss portrait you can open. The whole tile is the target rather than
 * just the picture — the name and the region belong to the same fight, and a
 * 56px circle is a mean thing to ask someone to hit. */
function BossButton({ boss, onOpen, children }: { boss: Boss; onOpen: (b: Boss) => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={() => onOpen(boss)}
      className="flex flex-col items-center rounded-lg px-1 py-1 text-center transition-colors hover:bg-white/5 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-amber"
    >
      {children}
    </button>
  );
}

/** The vertical link between two tiers, so the hero reads as one chain
 * rather than as three lists that happen to be stacked. */
function TierLink() {
  return (
    <div aria-hidden className="flex justify-center py-1">
      <span className="h-5 w-px bg-paper/20" />
    </div>
  );
}

/**
 * The page's one bold element: the whole boss progression drawn as the chain
 * it is — the eight Palpagos towers, then Panthalus, then the World Tree's
 * three in any order, then Astralym behind them.
 *
 * Read across a tier and it's how far the group has got together; read one
 * column and it's who still owes that fight a visit. The tiers are stacked
 * rather than listed because the gating is the information: you cannot reach
 * Astralym without all three of the run above it, and a flat list of thirteen
 * would say nothing about that.
 */
function BossChain({ players, onOpen }: { players: AchievementsPlayer[]; onOpen: (b: Boss) => void }) {
  const beaten = useMemo(() => players.map((p) => new Set(p.records.towers)), [players]);
  const extras = useMemo(
    () => extraTowerKeys(new Set(players.flatMap((p) => p.records.towers))),
    [players],
  );

  const clearedBy = (key: string) => players.filter((_, i) => beaten[i].has(key));
  const stateOf = (key: string): ClearState => {
    const n = clearedBy(key).length;
    return n === players.length && players.length > 0 ? "all" : n > 0 ? "some" : "none";
  };
  const dots = (boss: Boss) => (
    <ClearDots players={players} cleared={(p) => new Set(p.records.towers).has(boss.key)} what={bossLabel(boss)} />
  );

  // The wall is the earliest fight somebody still hasn't cleared — the next
  // thing the group can actually close out, rather than the hardest one.
  const wall = BOSS_CHAIN.find((b) => clearedBy(b.key).length < players.length);
  const wallCount = wall ? clearedBy(wall.key).length : 0;

  return (
    <section className="clip-notch-lg rounded-br-[10px] rounded-tl-[10px] border border-white/10 bg-ink px-4 py-5 text-paper lg:px-7 lg:py-6">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2 className="font-display text-lg font-extrabold">The tower run</h2>
        <p className="font-mono text-xs uppercase tracking-widest text-paper/45">In the order you meet them</p>
      </div>

      <p className="mt-4 font-mono text-[10px] uppercase tracking-widest text-paper/45">Palpagos Islands</p>
      <ol className="mt-3 grid grid-cols-4 gap-x-2 gap-y-5 lg:grid-cols-8">
        {PALPAGOS_TOWERS.map((tower, i) => (
          <li key={tower.key} className="relative flex flex-col items-center text-center">
            {/* The rail that makes the tiles one run. Drawn behind the
                portraits at their vertical centre, and only on the wide
                layout where they actually sit in one line. Each end stops at
                its own portrait rather than running off the edge of the set. */}
            <span
              aria-hidden
              className={cn(
                "pointer-events-none absolute top-7 hidden h-px bg-paper/15 lg:block",
                i === 0
                  ? "left-1/2 right-0"
                  : i === PALPAGOS_TOWERS.length - 1
                    ? "left-0 right-1/2"
                    : "left-0 right-0",
              )}
            />
            <BossButton boss={tower} onOpen={onOpen}>
              <BossPortrait boss={tower} state={stateOf(tower.key)} size="md" />
              {/* Two lines' worth of room whether the name needs it or not:
                  "Victor & Shadowbeak" wraps where the rest don't, and a single
                  tile dropping its region and dots half a line breaks the one
                  thing the run is for — reading straight across. */}
              <p className="mt-2 min-h-[2.05rem] font-display text-[13px] font-bold leading-tight">
                {bossLabel(tower)}
              </p>
              <p className="mt-0.5 font-mono text-[10px] uppercase tracking-wider text-paper/40">{tower.region}</p>
            </BossButton>
            <div className="mt-2 text-paper/30">{dots(tower)}</div>
          </li>
        ))}
      </ol>

      <TierLink />

      <div className="flex items-center justify-center gap-3">
        <BossButton boss={PANTHALUS} onOpen={onOpen}>
          <div className="flex items-center gap-3">
            <BossPortrait boss={PANTHALUS} state={stateOf(PANTHALUS.key)} size="md" />
            <div className="text-left">
              <p className="font-display text-[13px] font-bold leading-tight">{bossLabel(PANTHALUS)}</p>
              <p className="mt-0.5 font-mono text-[10px] uppercase tracking-wider text-paper/40">{PANTHALUS.region}</p>
              <div className="mt-1.5 flex text-paper/30">{dots(PANTHALUS)}</div>
            </div>
          </div>
        </BossButton>
      </div>

      <TierLink />

      <p className="text-center font-mono text-[10px] uppercase tracking-widest text-paper/45">
        World Tree — in any order
      </p>
      {/* Three columns rather than a wrapping row: they're a set you can take
          in any order, and a narrow screen breaking them 2 + 1 invents a
          sequence the game doesn't have. */}
      <ul className="mx-auto mt-3 grid max-w-md grid-cols-3 gap-x-3 gap-y-4">
        {WORLD_TREE_RUN.map((boss) => (
          <li key={boss.key} className="flex flex-col items-center text-center">
            <BossButton boss={boss} onOpen={onOpen}>
              <BossPortrait boss={boss} state={stateOf(boss.key)} size="sm" />
              <p className="mt-2 font-display text-[13px] font-bold leading-tight">{bossLabel(boss)}</p>
            </BossButton>
            <div className="mt-1.5 text-paper/30">{dots(boss)}</div>
          </li>
        ))}
      </ul>

      <TierLink />

      <div className="flex items-center justify-center gap-3">
        <BossButton boss={ASTRALYM} onOpen={onOpen}>
          <div className="flex items-center gap-3">
            <BossPortrait boss={ASTRALYM} state={stateOf(ASTRALYM.key)} size="lg" />
            <div className="text-left">
              <p className="font-display text-base font-extrabold leading-tight">{bossLabel(ASTRALYM)}</p>
              <p className="mt-0.5 font-mono text-[10px] uppercase tracking-wider text-paper/40">{ASTRALYM.region}</p>
              <div className="mt-1.5 flex text-paper/30">{dots(ASTRALYM)}</div>
            </div>
          </div>
        </BossButton>
      </div>

      {extras.length > 0 && (
        // A fight the chain doesn't know — a game update added one. Shown
        // rather than dropped, so the gap is visible and fixable.
        <p className="mt-5 border-t border-white/10 pt-4 font-mono text-xs text-paper/50">
          Also beaten, and not yet in palcon's boss list:{" "}
          {extras.map((k) => questLabel(k.replace(/^BOSS_BATTLE_NAME_/, ""))).join(", ")}
        </p>
      )}

      <p className="mt-5 border-t border-white/10 pt-4 text-sm text-paper/70">
        {players.length === 0
          ? "No players in the save yet."
          : wall
            ? wallCount === 0
              ? `Nobody has beaten ${bossLabel(wall)} yet.`
              : `${bossLabel(wall)} is the wall — ${wallCount} of ${players.length} ${wallCount === 1 ? "has" : "have"} cleared it.`
            : "Every fight down, for everyone."}
      </p>
    </section>
  );
}

/**
 * Raid bosses are summoned rather than found, and can be beaten again and
 * again — so they're the one place a count means more than a flag. The
 * denominator is the five summonable bosses; a save carrying one this list
 * doesn't know still gets a row.
 *
 * The counts are per player and must not be added up. Raids are fought as a
 * party and every participant's save records the same kill, so a guild that
 * downed one boss four-handed nine times reads as 9, 9, 8, 9 — summing those
 * into "35 defeated" invents twenty-six raids that never happened. The highest
 * single count is the honest floor: that many raids demonstrably took place.
 */
function RaidRoster({ players, onOpen }: { players: AchievementsPlayer[]; onOpen: (b: Boss) => void }) {
  const rows = useMemo(() => {
    const known = new Set(RAID_ROSTER.map((r) => r.key));
    const extra = [...new Set(players.flatMap((p) => Object.keys(p.records.raids)))]
      .filter((k) => !known.has(k))
      .sort()
      .map((key) => ({ key, palId: raidPalId(key) }));
    return [...RAID_ROSTER, ...extra];
  }, [players]);

  const mostClears = (key: string) => Math.max(0, ...players.map((p) => p.records.raids[key] ?? 0));
  const met = rows.filter((r) => mostClears(r.key) > 0).length;

  return (
    <section className="rounded-xl border border-ink/10 bg-white">
      <div className="flex items-baseline justify-between gap-3 border-b border-ink/5 px-5 py-4">
        <div>
          <h2 className="font-display text-base font-bold">Raid bosses</h2>
          <p className="mt-0.5 text-xs text-ink/50">
            Summoned at an altar, and beatable as often as you like. Everyone in the party records the same kill,
            so these are the most any one player has — not a server total.
          </p>
        </div>
        <span className="shrink-0 font-mono text-sm font-semibold tabular-nums text-legendary">
          {met}/{rows.length} met
        </span>
      </div>
      <ul className="divide-y divide-ink/5">
        {rows.map((raid) => {
          const most = mostClears(raid.key);
          return (
            <li key={raid.key} className="flex items-center gap-3 py-3 pl-5 pr-5">
              <button
                type="button"
                onClick={() => onOpen({ key: raid.key, palId: raid.palId })}
                className="-my-1 flex min-w-0 flex-1 items-center gap-3 rounded-lg py-1 text-left transition-colors hover:bg-ink/[0.03] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-red"
              >
              <img
                src={palIconUrl(raid.palId)}
                alt=""
                loading="lazy"
                className={cn(
                  "h-10 w-10 shrink-0 rounded-lg border border-ink/10 bg-paper object-contain",
                  most === 0 && "[filter:grayscale(1)_sepia(0.5)_brightness(0.97)_contrast(1.1)]",
                )}
              />
              <div className="min-w-0 flex-1">
                <p className="truncate font-display text-sm font-bold">{recordName(raid.palId)}</p>
                <p className="text-xs text-ink/45">
                  {most === 0
                    ? "Never summoned on this server"
                    : `${most.toLocaleString()} ${most === 1 ? "clear" : "clears"}, the most by one player`}
                </p>
              </div>
              </button>
              <div className="text-ink/20">
                <ClearDots
                  players={players}
                  cleared={(p) => (p.records.raids[raid.key] ?? 0) > 0}
                  what={recordName(raid.palId)}
                />
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** One number and what it counts, for the grid inside an expanded player. */
function Stat({ label, value, hint }: { label: string; value: number; hint?: string }) {
  return (
    <div title={hint}>
      <p className="font-mono text-lg font-semibold tabular-nums">{value.toLocaleString()}</p>
      <p className="text-[11px] leading-tight text-ink/45">{label}</p>
    </div>
  );
}

/** A labelled block inside an expanded player, so the panel reads as sections
 * rather than as one run-on column of text. */
function Detail({ label, note, children }: { label: string; note?: string; children: ReactNode }) {
  return (
    <div>
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/35">
        {label}
        {note && <span className="ml-1 normal-case tracking-normal text-ink/30">{note}</span>}
      </p>
      {children}
    </div>
  );
}

/**
 * A portrait chip: what something is, with its face on it. Used for the things
 * the icon set actually covers — towers, raid bosses, the field alphas whose
 * spawner id names a species.
 *
 * `dim` sepias the portrait for something outstanding rather than done, which
 * is the same reveal the tower run and the Paldex's wanted posters use.
 */
function BossChip({
  boss,
  palId,
  label,
  sub,
  dim,
}: {
  /** Pass a chain entry when the chip might be the Forbidden Laboratory, which
   * draws as a silhouette rather than as the pal whose outline it borrows. */
  boss?: Boss;
  palId?: string;
  label: string;
  sub?: string;
  dim?: boolean;
}) {
  const lab = boss ? isLaboratory(boss) : false;
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-ink/10 bg-white py-1 pl-1 pr-3">
      {/* Ring and ground on the wrapper, filter on the image — see
          BossPortrait for why the two can't share an element. */}
      <span
        className={cn(
          "flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-full border border-ink/10",
          lab ? "bg-[#1b0f0c]" : "bg-paper",
        )}
      >
        <img
          src={palIconUrl(palId ?? boss?.palId ?? "")}
          alt=""
          loading="lazy"
          className={cn(
            "h-full w-full object-contain",
            !lab && dim && "[filter:grayscale(1)_sepia(0.5)_brightness(0.97)_contrast(1.1)]",
            lab &&
              "scale-[0.72] [filter:brightness(0)_drop-shadow(1px_0_0_#E8491D)_drop-shadow(-1px_0_0_#E8491D)_drop-shadow(0_1px_0_#E8491D)_drop-shadow(0_-1px_0_#E8491D)]",
          )}
        />
      </span>
      <span className="text-sm leading-tight">
        {label}
        {sub && <span className="ml-1 font-mono text-xs text-ink/45">{sub}</span>}
      </span>
    </span>
  );
}

/**
 * A bounty target, as a name chip rather than a portrait.
 *
 * Deliberately not a BossChip: the pal icon set covers pals, and only 11 of the
 * 34 human targets happen to have a portrait in it. A grid two thirds full of
 * blank squares reads as broken artwork, not as decoration — so the whole
 * roster gets the same typographic treatment and none of it looks missing.
 */
function BountyChip({ name }: { name: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-ink/10 bg-white px-2.5 py-1 text-sm">
      <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-brand-red/50" />
      {name}
    </span>
  );
}

/**
 * A player's own record. Collapsed it's the five figures worth comparing down
 * the column; expanded it says which specific things are still outstanding,
 * which is the only question the summary can't answer.
 */
function PlayerRow({ player }: { player: AchievementsPlayer }) {
  const [open, setOpen] = useState(false);
  const r = player.records;
  const field = useMemo(() => splitFieldBosses(r.fieldBosses), [r.fieldBosses]);
  const bosses = bossesCleared(r);
  const bossesLeft = BOSS_CHAIN.filter((b) => !new Set(r.towers).has(b.key));
  const bountiesDown = new Set(field.bounties);
  const quests = mainQuests(r.quests);
  const repeatBosses = BOSS_CHAIN.map((b) => {
    const c = towerClears(r, b.key);
    return { ...b, times: c.normal + c.hard };
  }).filter((b) => b.times > 1);
  // This player's own raid clears, roster order, skipping the ones they've
  // never beaten — a chip reading "×0" is just a slower way of saying nothing.
  const raids = useMemo(
    () =>
      Object.entries(r.raids)
        .filter(([, count]) => count > 0)
        .map(([key, count]) => ({ key, count, palId: raidPalId(key) }))
        .sort((a, b) => b.count - a.count),
    [r.raids],
  );

  return (
    <li>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-ink/[0.03]"
      >
        <span
          aria-hidden
          className="h-7 w-1 shrink-0 rounded-full"
          style={{ backgroundColor: playerColor(player.uid) }}
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-display text-sm font-bold">{player.nickname || "Unnamed"}</span>
          <span className="text-xs text-ink/45">Level {player.level}</span>
        </span>
        {/* Narrow screens can't hold three figures, so they get the one that
            orders the list — a collapsed row with only a name and a level
            gives no reason to open it. */}
        <span className="shrink-0 font-mono text-sm tabular-nums sm:hidden" title="Boss battles in the chain beaten">
          {bosses}
          <span className="text-ink/35">/{BOSS_CHAIN.length}</span>
          <span className="ml-1 text-[11px] font-normal text-ink/45">bosses</span>
        </span>
        <span className="hidden shrink-0 gap-5 font-mono text-sm tabular-nums sm:flex">
          <span title="Boss battles in the chain beaten">
            {bosses}
            <span className="text-ink/35">/{BOSS_CHAIN.length}</span>
            <span className="ml-1 text-[11px] font-normal text-ink/45">bosses</span>
          </span>
          <span title="Named bounty targets taken down">
            {bountiesDown.size}
            <span className="text-ink/35">/{BOUNTY_ROSTER.length}</span>
            <span className="ml-1 text-[11px] font-normal text-ink/45">bounties</span>
          </span>
          <span title="Main-story quests completed">
            {quests.length}
            <span className="ml-1 text-[11px] font-normal text-ink/45">quests</span>
          </span>
        </span>
        <ChevronDown
          className={cn("h-4 w-4 shrink-0 text-ink/35 transition-transform motion-reduce:transition-none", open && "rotate-180")}
        />
      </button>

      {open && (
        <div className="space-y-4 border-t border-ink/5 bg-ink/[0.02] px-5 py-4">
          <p className="text-xs text-ink/50">{seenSentence(player)}</p>

          <div className="grid grid-cols-3 gap-x-4 gap-y-3 sm:grid-cols-5 lg:grid-cols-6">
            <Stat label="bosses beaten" value={bosses} />
            <Stat label="bounties down" value={bountiesDown.size} />
            <Stat
              label="field alphas"
              value={field.alphaCount}
              hint="Since the game last reset field boss respawns — not a lifetime total."
            />
            <Stat
              label="raid clears"
              value={Object.values(r.raids).reduce((a, b) => a + b, 0)}
              hint="This player's own clears across every raid boss. Party members each record a shared kill, so this doesn't add up across the server."
            />
            <Stat label="dungeons cleared" value={r.dungeonsCleared + r.fixedDungeonsCleared} />
            <Stat label="camps taken" value={r.campsConquered} />
            <Stat label="main quests" value={quests.length} />
            <Stat label="areas found" value={r.areas} />
            <Stat label="map points" value={r.fastTravel} hint="Fast travel statues, dungeon mouths and other unlocked map points." />
            <Stat label="effigies" value={r.relics} />
            <Stat label="notes read" value={r.notes} />
            <Stat label="species caught" value={r.tribesCaptured} />
            <Stat label="predators" value={r.predatorsDefeated} />
            <Stat label="oil rigs" value={r.oilrigsCleared} />
            <Stat label="awakenings" value={r.awakenings} />
          </div>

          {/* The arena is a ladder, so the rank held says more than a count.
              Sits with the story flag because both are status, not tallies. */}
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-ink/50">
            <span>
              Arena rank:{" "}
              <span className="font-mono font-semibold text-ink/70">{arenaRank(r.arenaRanks) ?? "unranked"}</span>
            </span>
            {r.gameCleared && <span className="font-semibold text-brand-red">Story complete</span>}
          </div>

          <Detail label="Still standing">
            {bossesLeft.length === 0 ? (
              <p className="text-sm text-ink/60">All {BOSS_CHAIN.length} down.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {bossesLeft.map((b) => (
                  <BossChip key={b.key} boss={b} label={bossLabel(b)} sub={b.region} dim />
                ))}
              </div>
            )}
            {/* Only worth saying once somebody has gone back for a second
                run — otherwise every tower reads "×1" and says nothing. */}
            {repeatBosses.length > 0 && (
              <p className="mt-2 text-xs text-ink/45">
                Repeat clears: {repeatBosses.map((b) => `${bossLabel(b)} ×${b.times}`).join(", ")}
              </p>
            )}
          </Detail>

          {raids.length > 0 && (
            <Detail label="Raid bosses beaten">
              <div className="flex flex-wrap gap-2">
                {raids.map((raid) => (
                  <BossChip key={raid.key} palId={raid.palId} label={recordName(raid.palId)} sub={`×${raid.count}`} />
                ))}
              </div>
            </Detail>
          )}

          {field.alphasNamed.length > 0 && (
            <Detail
              label="Field alphas beaten"
              note={`(${field.alphasNamed.length} of ${field.alphaCount} name themselves in the save)`}
            >
              <div className="flex flex-wrap gap-2">
                {field.alphasNamed.map((a) => (
                  <BossChip key={a.key} palId={a.palId} label={a.name} />
                ))}
              </div>
            </Detail>
          )}

          <Detail label="Bounty targets still out there">
            {bountiesDown.size === BOUNTY_ROSTER.length ? (
              <p className="text-sm text-ink/60">Every named target down.</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {BOUNTY_ROSTER.filter((k) => !bountiesDown.has(k)).map((k) => (
                  <BountyChip key={k} name={recordName(k)} />
                ))}
              </div>
            )}
          </Detail>

          {/* The completed array is in the order they were finished, so the
              last entry is where this player has got to in the story. */}
          {quests.length > 0 && (
            <p className="text-xs text-ink/45">Latest main quest: {questLabel(quests[quests.length - 1])}</p>
          )}
        </div>
      )}
    </li>
  );
}

export function ServerAchievements() {
  const { serverID } = useParams();
  const id = Number(serverID);

  const query = useQuery({
    queryKey: ["server-achievements", id],
    queryFn: () => api.serverAchievements(id),
    retry: false,
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });
  const infoQuery = useQuery({
    queryKey: ["server-info", id],
    queryFn: () => api.serverInfo(id),
    retry: false,
    staleTime: 15_000,
  });

  const notConfigured =
    query.error instanceof ApiError && query.error.message.includes("no save path configured");
  const hasData = query.data !== undefined;
  const players = query.data?.players ?? [];

  // One dialog for both rosters — a raid and a tower answer the same question
  // ("what is this fight, and who here has done it"), and the raid key tells
  // the dialog which table to read and whether to show clear counts.
  const [fight, setFight] = useState<Boss | null>(null);
  const fightIsRaid = fight ? fight.key.startsWith("PalSummon_") : false;

  // The records live in Players/*.sav, one file per player. A save path that
  // only mounts Level.sav parses fine and yields players with nothing beaten,
  // which would read as a server where nobody has done anything.
  const recordsMissing =
    players.length > 0 && players.every((p) => p.records.towers.length === 0 && p.records.quests.length === 0);

  return (
    <div className="pb-24">
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Achievements</h1>
          <p className="text-sm text-ink/60">What this server has beaten, and what's still standing</p>
        </div>
      </header>

      <div className="mx-auto max-w-5xl space-y-4 p-4 lg:space-y-6 lg:p-8">
        {!hasData && query.isFetching && <SaveReadProgress />}

        {notConfigured && !hasData && <SavePathSetup />}

        {!hasData &&
          query.isError &&
          !notConfigured &&
          (infoQuery.isError ? (
            <ServerUnreachable />
          ) : (
            <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-ink/70">
              The save could not be read. Refresh to try again.
            </p>
          ))}

        {hasData && query.isFetching && <SaveUpdatingBanner />}

        {query.data && (
          <>
            {recordsMissing && (
              <p className="rounded-lg border border-brand-amber/50 bg-brand-amber/10 px-4 py-3 text-sm text-ink/70">
                No completion records were found in the save. They live in the world folder's{" "}
                <code className="font-mono">Players/*.sav</code> files, so make sure the server's save path mounts
                the whole folder, not just Level.sav.
              </p>
            )}

            <BossChain players={players} onOpen={setFight} />

            <RaidRoster players={players} onOpen={setFight} />

            <section className="rounded-xl border border-ink/10 bg-white">
              <div className="border-b border-ink/5 px-5 py-4">
                <h2 className="font-display text-base font-bold">By player</h2>
                <p className="mt-0.5 text-xs text-ink/45">
                  Field alphas respawn, so that count is what each player has beaten since the game last reset
                  them — everything else is permanent.
                </p>
              </div>
              {players.length === 0 ? (
                <p className="px-5 py-6 text-sm text-ink/60">No players in the save yet.</p>
              ) : (
                <ul className="divide-y divide-ink/5">
                  {players.map((p) => (
                    <PlayerRow key={p.uid} player={p} />
                  ))}
                </ul>
              )}
            </section>
          </>
        )}
      </div>

      <BossFightDialog
        boss={fight}
        fight={fight ? (fightIsRaid ? raidFight(fight.key) : towerFight(fight.key)) : undefined}
        players={players}
        kind={fightIsRaid ? "raid" : "tower"}
        onClose={() => setFight(null)}
      />
    </div>
  );
}
