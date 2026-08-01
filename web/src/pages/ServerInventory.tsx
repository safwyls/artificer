import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, RefreshCw, Search } from "lucide-react";
import { api, ApiError, type Character, type ItemContainer, type ItemSlot, type PlayerInventory } from "../lib/api";
import { initials, playerColor } from "../lib/palette";
import {
  CATEGORIES,
  GEAR_SLOTS,
  durabilityFraction,
  equipSlot,
  itemCategory,
  itemIconUrl,
  itemName,
  rarityColor,
  stackWeight,
} from "../lib/items";
import { agoLabel, seenPhrase } from "../lib/time";
import { cn } from "../lib/utils";
import { ItemDetailDialog } from "../components/ItemDetailDialog";
import { SavePathSetup } from "../components/SavePathSetup";
import { SaveReadProgress } from "../components/SaveReadProgress";
import { SaveUpdatingBanner } from "../components/SaveUpdatingBanner";
import { ServerUnreachable } from "../components/ServerUnreachable";
import { Input } from "../components/ui/input";
import { Select } from "../components/ui/select";

// ---------------------------------------------------------------------------
// The containers, in the order a player would open them.
//
// `grid` says whether the container's empty slots are worth drawing. A backpack
// has a capacity the player feels — 45 of 45 used is the difference between
// "carrying a lot" and "can't pick anything up" — so it's drawn whole. Key
// items sit in a 230-slot container nobody ever fills; drawing 190 empty wells
// would say something about the player that isn't true.
// ---------------------------------------------------------------------------

const BAGS = [
  { role: "common", label: "Carrying", grid: "full" as const },
  { role: "essential", label: "Key items", grid: "compact" as const },
  { role: "drop", label: "Death drop", grid: "full" as const },
];

/** Worn rather than carried; racked by buildRacks, which splits the armour
 * container into its individual sockets. */
const EQUIPPED_ROLES = ["weapons", "equipment", "food"];

/** Every container a player has, for the header's item/weight/gold totals. */
const ALL_ROLES = [...BAGS.map((b) => b.role), ...EQUIPPED_ROLES];

/** Money is an item in the save, sitting in a real backpack slot — it stays in
 * the grid where the game puts it, and also rides up to the header, where "how
 * much gold is this player sitting on" is the question actually being asked. */
const MONEY_ID = "Money";

interface Filters {
  query: string;
  category: string;
}

function filtersActive(f: Filters): boolean {
  return Boolean(f.query.trim()) || Boolean(f.category);
}

function matchSlot(slot: ItemSlot, f: Filters): boolean {
  if (f.category && itemCategory(slot.itemId) !== f.category) return false;
  const q = f.query.trim().toLowerCase();
  if (!q) return true;
  return (
    itemName(slot.itemId).toLowerCase().includes(q) ||
    slot.itemId.toLowerCase().includes(q) ||
    (slot.eggSpecies ?? "").toLowerCase().includes(q)
  );
}

// ---------------------------------------------------------------------------

function SlotCell({
  slot,
  dimmed,
  size,
  onOpen,
}: {
  slot: ItemSlot | null;
  /** A filter is on and this slot isn't a match — kept in place so the bag's
   * layout survives filtering, just pushed to the back. */
  dimmed: boolean;
  /** Bag cells stretch to fill their grid column; equipped cells are fixed, so
   * a four-slot weapon rack doesn't span the page. */
  size: string;
  onOpen: (slot: ItemSlot) => void;
}) {
  if (!slot) {
    return <div className={cn("rounded-md border border-white/[0.04] bg-black/25", size)} />;
  }

  const color = rarityColor(slot.itemId);
  const wear = durabilityFraction(slot.itemId, slot.durability);
  const icon = itemIconUrl(slot.itemId);

  return (
    <button
      onClick={() => onOpen(slot)}
      title={`${itemName(slot.itemId)}${slot.count > 1 ? ` ×${slot.count.toLocaleString()}` : ""}`}
      // content-visibility lets the browser skip painting slots scrolled out
      // of view — a busy server is thousands of cells.
      className={cn(
        "group relative rounded-md border bg-ink-light transition-colors [content-visibility:auto]",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-amber",
        size,
        dimmed ? "opacity-20" : "hover:bg-[#4A403A]",
      )}
      style={{ borderColor: color ? `${color}55` : "rgba(255,255,255,0.07)" }}
    >
      {icon && (
        <img
          src={icon}
          alt={itemName(slot.itemId)}
          className="absolute inset-1 h-[calc(100%-0.5rem)] w-[calc(100%-0.5rem)] object-contain"
          loading="lazy"
          decoding="async"
          // An item added by a game update has no vendored icon; the framed
          // cell still reads, so drop the broken image rather than show it.
          onError={(e) => {
            e.currentTarget.style.visibility = "hidden";
          }}
        />
      )}
      {slot.count > 1 && (
        <span className="absolute bottom-0 right-0.5 font-mono text-[10px] font-bold leading-tight text-paper drop-shadow-[0_1px_1px_rgba(0,0,0,0.9)]">
          {slot.count > 9999 ? `${Math.floor(slot.count / 1000)}k` : slot.count}
        </span>
      )}
      {wear !== undefined && (
        <span className="absolute inset-x-1 bottom-0.5 h-[2px] rounded-full bg-black/50">
          <span
            className="block h-full rounded-full"
            style={{
              width: `${wear * 100}%`,
              backgroundColor: wear > 0.5 ? "#4A9D7C" : wear > 0.2 ? "#F2A93B" : "#E8491D",
            }}
          />
        </span>
      )}
    </button>
  );
}

/** A container drawn as its real grid: slot 3 is in position 3, and a gap is a
 * gap. `compact` containers drop their unfilled tail (see BAGS). */
function SlotGrid({
  container,
  compact,
  filters,
  onOpen,
}: {
  container: ItemContainer;
  compact: boolean;
  filters: Filters;
  onOpen: (slot: ItemSlot) => void;
}) {
  const cells = useMemo(() => {
    const byIndex = new Map(container.slots.map((s) => [s.slot, s]));
    if (compact) return container.slots;
    const last = container.slots.reduce((m, s) => Math.max(m, s.slot), -1);
    // Trust whichever is larger: a save that under-reports capacity still
    // has to have room for the slots it actually holds.
    const size = Math.max(container.size, last + 1);
    return Array.from({ length: size }, (_, i) => byIndex.get(i) ?? null);
  }, [container, compact]);

  const active = filtersActive(filters);

  return (
    // Capped rather than fluid: past ~18 columns a bag stops reading as a bag
    // and starts reading as a filmstrip, and an ultrawide monitor would draw
    // one 25-slot row with a ragged orphan under it.
    <div
      className={cn(
        "grid max-w-4xl gap-1 rounded-xl bg-ink p-2",
        "grid-cols-[repeat(auto-fill,minmax(2.75rem,1fr))] sm:grid-cols-[repeat(auto-fill,minmax(3.25rem,1fr))]",
      )}
    >
      {cells.map((slot, i) => (
        <SlotCell
          key={slot ? `${slot.slot}-${slot.itemId}` : `empty-${i}`}
          slot={slot}
          size="aspect-square"
          dimmed={Boolean(slot) && active && !matchSlot(slot!, filters)}
          onOpen={onOpen}
        />
      ))}
    </div>
  );
}

/** One equipped item, or an empty socket. `caption` names the socket, which is
 * the whole reason gear is grouped by kind — "weapon 2" would say nothing, so
 * the weapon rack passes none. */
interface RackEntry {
  slot: ItemSlot | null;
  caption: string;
  key: string;
}

/**
 * A vertical rack of equipment: one narrow ink column per group, cells stacked.
 * Racked rather than laid in a row because that's how the game shows them, and
 * because a five-socket column stands about as tall as the stat list beside it.
 */
function SlotRack({
  label,
  entries,
  filters,
  onOpen,
}: {
  label: string;
  entries: RackEntry[];
  filters: Filters;
  onOpen: (slot: ItemSlot) => void;
}) {
  const active = filtersActive(filters);
  if (entries.length === 0) return null;

  return (
    <div>
      <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-ink/40">{label}</p>
      <div className="flex w-fit flex-col gap-1 rounded-xl bg-ink p-2">
        {entries.map(({ slot, caption, key }) => (
          <span key={key} className="flex w-12 shrink-0 flex-col items-center gap-0.5">
            <SlotCell
              slot={slot}
              size="h-12 w-12 shrink-0"
              dimmed={Boolean(slot) && active && !matchSlot(slot!, filters)}
              onOpen={onOpen}
            />
            {caption && (
              <span className="w-full truncate text-center text-[9px] leading-none text-paper/50">{caption}</span>
            )}
          </span>
        ))}
      </div>
    </div>
  );
}

/** Racks in the order they're shown, built from the player's containers.
 *
 * The armour container mixes every worn thing into one list, so it's split
 * back into the five one-of-a-kind sockets and the accessories. The five
 * always draw, empty or not — an unarmoured level 70 is worth seeing. */
function buildRacks(player: PlayerInventory): { label: string; entries: RackEntry[] }[] {
  const worn = player.inventory.equipment?.slots ?? [];
  const bySlot = new Map(worn.map((s) => [equipSlot(s.itemId) ?? "", s]));

  const gear: RackEntry[] = GEAR_SLOTS.map((socket) => {
    const slot = bySlot.get(socket) ?? null;
    return {
      slot,
      caption: SLOT_CAPTIONS[socket],
      key: slot ? `${slot.slot}-${slot.itemId}` : `empty-${socket}`,
    };
  });

  const accessories: RackEntry[] = worn
    .filter((s) => equipSlot(s.itemId) === "Accessory")
    .map((slot) => ({ slot, caption: "accessory", key: `${slot.slot}-${slot.itemId}` }));

  // A plain vertical run, no captions — its position in the rack is its name.
  const straight = (role: string): RackEntry[] =>
    (player.inventory[role]?.slots ?? []).map((slot) => ({
      slot,
      caption: "",
      key: `${slot.slot}-${slot.itemId}`,
    }));

  return [
    { label: "Weapons", entries: straight("weapons") },
    { label: "Gear", entries: gear },
    { label: "Accessories", entries: accessories },
    { label: "Food", entries: straight("food") },
  ];
}

/** Short enough to sit under a 48px cell. */
const SLOT_CAPTIONS: Record<string, string> = {
  Head: "head",
  Body: "body",
  Shield: "shield",
  Glider: "glider",
  SphereModule: "sphere",
};

// ---------------------------------------------------------------------------
// The build. The game's character screen shows finished numbers — Health 2050,
// Attack 100, Work Speed 653 — that it computes at runtime from base values,
// level and gear; the save holds none of them. What it does hold is how the
// player spent their points, which the game only shows one stat at a time.
// ---------------------------------------------------------------------------

/** The six the game itself puts on the stat panel, in its order. Keys are the
 * extractor's stat names; the labels are what the game calls them. */
const CORE_STATS: [string, string][] = [
  ["Max HP", "Health"],
  ["Max SP", "Stamina"],
  ["Attack", "Attack"],
  ["Carry Weight", "Weight"],
  ["Capture Rate", "Capture"],
  ["Work Speed", "Work speed"],
];

const CORE_KEYS = new Set(CORE_STATS.map(([key]) => key));

/**
 * The relic-granted stats, in a fixed order with short labels.
 *
 * Order is the list's, not the values': sorting by size gave every player a
 * different arrangement, so two sections couldn't be read against each other —
 * which is the only reason to show a dozen small numbers at all. The order
 * matches the extractor's, which is palworld-save-pal's.
 *
 * "Stamina cost" rather than "Stamina": the core panel above already uses that
 * word for the pool, and this one is about how fast it drains.
 */
const ADVENTURE_STATS: [string, string][] = [
  ["Movement Speed", "Move"],
  ["Stamina Cost Reduction", "Stamina cost"],
  ["Jump Power", "Jump"],
  ["Climb Speed", "Climb"],
  ["Swim Speed", "Swim"],
  ["Glide Speed", "Glide"],
  ["Hunger Reduction", "Hunger"],
  ["Food Spoilage Reduction", "Spoilage"],
  ["Ailment Resistance", "Ailments"],
  ["Sphere Homing", "Homing"],
  ["EXP Bonus", "EXP"],
  ["Rainbow Passive Rate", "Rainbow"],
];

const ADVENTURE_KEYS = new Set(ADVENTURE_STATS.map(([key]) => key));

/** Lifetime EXP runs to eight digits, which would dominate the header line;
 * "25.8M" carries the same meaning at a glance. */
function compactExp(exp: number): string {
  if (exp >= 1_000_000) return `${(exp / 1_000_000).toFixed(1)}M`;
  if (exp >= 1_000) return `${Math.round(exp / 1_000)}k`;
  return String(exp);
}

/** The highest single core-stat investment anyone on the server reached, which
 * every player's bars are drawn against. One shared ceiling is what makes two
 * sections comparable; per-player normalising would make an empty build look
 * identical to a maxed one. Never below 1, so a server of fresh characters
 * doesn't divide by zero. */
function coreStatScale(players: PlayerInventory[]): number {
  let max = 1;
  for (const p of players) {
    if (!p.character) continue;
    for (const [key] of CORE_STATS) {
      const total = (p.character.statusPoints[key] ?? 0) + (p.character.exStatusPoints[key] ?? 0);
      if (total > max) max = total;
    }
  }
  return max;
}

function StatBar({
  label,
  points,
  exPoints,
  max,
}: {
  label: string;
  points: number;
  exPoints: number;
  /** The highest total any player on this server reached in this stat, so the
   * bars compare across sections instead of each one normalising to itself. */
  max: number;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-20 shrink-0 text-xs text-ink/55">{label}</span>
      <span className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-ink/10">
        <span className="flex h-full">
          <span className="h-full bg-ink/70" style={{ width: `${(points / max) * 100}%` }} />
          {/* The Ex pool continues the same bar rather than getting its own:
              total investment is the full length, split by what paid for it. */}
          <span className="h-full bg-brand-amber" style={{ width: `${(exPoints / max) * 100}%` }} />
        </span>
      </span>
      {/* The two pools stay separate figures. Printing the total next to the
          Ex count read as a sum of the two ("12 +12" for nothing but 12 Ex
          points), which is the opposite of what it meant. */}
      <span className="w-14 shrink-0 text-right font-mono text-xs text-ink">
        {points || <span className="text-ink/25">0</span>}
        {exPoints > 0 && <span className="ml-1 text-brand-amber">+{exPoints}</span>}
      </span>
    </div>
  );
}

function BuildPanel({ character, scale }: { character: Character; scale: number }) {
  // Every stat, every player, same order — a row that's present for one player
  // and missing for the next can't be compared at a glance. Zeroes are shown
  // greyed rather than dropped for the same reason.
  const adventure = ADVENTURE_STATS.map(
    ([key, label]) => [label, character.statusPoints[key] ?? 0] as const,
  );
  const hasAdventure = adventure.some(([, value]) => value > 0);

  // Anything the extractor reported that this list doesn't know about — a stat
  // added by a game update. Shown rather than swallowed, so a missing mapping
  // surfaces as a visible oddity instead of silently vanishing.
  const unknown = Object.entries(character.statusPoints).filter(
    ([key, value]) => !CORE_KEYS.has(key) && !ADVENTURE_KEYS.has(key) && value > 0,
  );

  return (
    // A stat list is a ranking, and a ranking reads down one column. Capped
    // so an ultrawide doesn't stretch a bar for a value under 40 across half
    // the screen.
    <div className="min-w-0 flex-1 basis-80 md:max-w-md">
      <div className="mb-2 flex items-baseline gap-2">
        <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">Build</p>
        {character.unusedStatusPoints > 0 && (
          <p className="font-mono text-[11px] font-semibold text-brand-red">
            {character.unusedStatusPoints} unspent
          </p>
        )}
      </div>

      <div className="space-y-1.5">
        {CORE_STATS.map(([key, label]) => (
          <StatBar
            key={key}
            label={label}
            points={character.statusPoints[key] ?? 0}
            exPoints={character.exStatusPoints[key] ?? 0}
            max={scale}
          />
        ))}
      </div>

      {(hasAdventure || unknown.length > 0) && (
        // Two columns, label left and figure right, filled top-to-bottom then
        // across so each column is a contiguous run of the fixed order rather
        // than every other entry.
        <div
          className="mt-3 grid grid-flow-col gap-x-6 gap-y-1 border-t border-ink/10 pt-2.5"
          style={{ gridTemplateRows: `repeat(${Math.ceil((adventure.length + unknown.length) / 2)}, minmax(0, auto))` }}
        >
          {adventure.map(([label, value]) => (
            <div key={label} className="flex items-baseline justify-between gap-2">
              <span className={cn("truncate text-xs", value > 0 ? "text-ink/45" : "text-ink/25")}>{label}</span>
              <span className={cn("shrink-0 font-mono text-xs", value > 0 ? "text-ink/70" : "text-ink/25")}>
                {value}
              </span>
            </div>
          ))}
          {unknown.map(([key, value]) => (
            <div key={key} className="flex items-baseline justify-between gap-2" title="Stat added by a game update">
              <span className="truncate text-xs text-brand-amber">{key}</span>
              <span className="shrink-0 font-mono text-xs text-brand-amber">{value}</span>
            </div>
          ))}
        </div>
      )}

      <p className="mt-2 text-[11px] leading-snug text-ink/35">
        Points invested. The game adds level and equipment on top, so these
        aren't the totals on a player's own stat screen.
      </p>
    </div>
  );
}

/**
 * HP, shield and hunger, on the player's own bar under the level line — it's
 * who they are rather than how they're kitted out, and putting it in the
 * header means it still reads when a section is collapsed.
 *
 * Only hunger gets a bar; it's the one figure here that really is out of 100.
 */
function ConditionStrip({ character }: { character: Character }) {
  const stomach = Math.max(0, Math.min(100, character.stomach));
  return (
    <span className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs text-ink/40">
      <span>
        HP <span className="font-bold text-[#4A9D7C]">{character.hp.toLocaleString()}</span>
      </span>
      {character.shield > 0 && (
        <span>
          Shield <span className="font-bold text-pal-blue">{character.shield.toLocaleString()}</span>
        </span>
      )}
      <span className="flex items-center gap-1.5">
        Hunger
        <span className="h-1.5 w-14 overflow-hidden rounded-full bg-ink/15">
          <span className="block h-full rounded-full bg-brand-amber" style={{ width: `${stomach}%` }} />
        </span>
        <span className="font-bold text-ink/70">{Math.round(stomach)}%</span>
      </span>
      {character.foodBuff && (
        // No countdown: the recorded seconds stopped being true the moment the
        // save was written, and a ticking number would imply otherwise.
        <span className="flex items-center gap-1.5">
          Buff
          <img
            src={itemIconUrl(character.foodBuff)}
            alt=""
            className="h-4 w-4 object-contain"
            loading="lazy"
            decoding="async"
            onError={(e) => {
              e.currentTarget.style.visibility = "hidden";
            }}
          />
          <span className="font-bold text-ink/70">{itemName(character.foodBuff)}</span>
        </span>
      )}
    </span>
  );
}

/** Everything the header line reports, computed once per player. */
function summarize(player: PlayerInventory) {
  let weight = 0;
  let money = 0;
  const slots: ItemSlot[] = [];
  for (const role of ALL_ROLES) {
    for (const slot of player.inventory[role]?.slots ?? []) {
      slots.push(slot);
      weight += stackWeight(slot.itemId, slot.count);
      if (slot.itemId === MONEY_ID) money += slot.count;
    }
  }
  return { items: slots.length, weight, money, slots };
}

function PlayerSection({
  player,
  open,
  onToggle,
  filters,
  statScale,
  onOpen,
}: {
  player: PlayerInventory;
  open: boolean;
  onToggle: () => void;
  filters: Filters;
  /** Shared ceiling for the build bars — see StatBar. */
  statScale: number;
  onOpen: (slot: ItemSlot, location: string) => void;
}) {
  const color = playerColor(player.uid);
  const { items, weight, money, slots } = useMemo(() => summarize(player), [player]);
  const active = filtersActive(filters);
  const matches = active ? slots.filter((s) => matchSlot(s, filters)).length : items;

  // A filter that nothing in this player's bags satisfies hides the player,
  // rather than leaving an all-dimmed section to scroll past.
  if (active && matches === 0) return null;

  // A rack survives a filter if anything in it matches; the gear rack keeps
  // its empty sockets, so it's judged on the items it actually holds.
  const racks = buildRacks(player).filter(({ entries }) => {
    const held = entries.filter((e) => e.slot);
    if (held.length === 0) return false;
    return !active || held.some((e) => matchSlot(e.slot!, filters));
  });

  const bags = BAGS.filter(({ role }) => {
    const container = player.inventory[role];
    if (!container) return false;
    // The backpack is worth showing empty — an empty one is a finding. The
    // other bags aren't.
    if (!container.slots.length && role !== "common") return false;
    return !active || container.slots.some((s) => matchSlot(s, filters));
  });

  return (
    <section className="overflow-hidden rounded-2xl border border-ink/10 bg-white/70">
      <button
        onClick={onToggle}
        className="flex w-full items-center gap-3 px-5 py-4 text-left transition-colors hover:bg-ink/5"
        aria-expanded={open}
      >
        <span
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full font-display text-sm font-bold"
          style={{ backgroundColor: `${color}33`, color }}
        >
          {initials(player.nickname || "?")}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-display text-base font-bold">{player.nickname || player.uid}</h2>
          <p className="flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-ink/40">
            <span>Lv.{player.level}</span>
            {player.character && player.character.exp > 0 && (
              <>
                <span aria-hidden>·</span>
                <span>{compactExp(player.character.exp)} xp</span>
              </>
            )}
            <span aria-hidden>·</span>
            <span>{active ? `${matches} of ${items}` : items} items</span>
            <span aria-hidden>·</span>
            <span>{weight.toLocaleString(undefined, { maximumFractionDigits: 1 })} kg</span>
            {money > 0 && (
              <>
                <span aria-hidden>·</span>
                <span className="text-brand-amber">{money.toLocaleString()} gold</span>
              </>
            )}
          </p>
          {player.character && (
            <div className="mt-1">
              <ConditionStrip character={player.character} />
            </div>
          )}
        </div>
        {seenPhrase(player) && (
          <span className="hidden shrink-0 font-mono text-xs text-ink/35 sm:inline">{seenPhrase(player)}</span>
        )}
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-ink/40 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="space-y-5 border-t border-ink/10 p-5">
          {(racks.length > 0 || player.character) && (
            // What they're wearing, then how they built themselves. Condition
            // lives on the player's bar above, not here.
            <div className="flex flex-wrap items-start gap-x-10 gap-y-5">
              {racks.length > 0 && (
                <div className="flex gap-3">
                  {racks.map(({ label, entries }) => (
                    <SlotRack
                      key={label}
                      label={label}
                      entries={entries}
                      filters={filters}
                      onOpen={(slot) => onOpen(slot, label)}
                    />
                  ))}
                </div>
              )}
              {player.character && <BuildPanel character={player.character} scale={statScale} />}
            </div>
          )}

          {bags.map(({ role, label, grid }) => {
            const container = player.inventory[role];
            const used = container.slots.length;
            return (
              <div key={role}>
                <p className="mb-1.5 flex items-baseline gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-ink/40">
                  {label}
                  <span className="font-mono normal-case tracking-normal text-ink/30">
                    {grid === "full" && container.size > 0 ? `${used} / ${container.size} slots` : `${used}`}
                  </span>
                </p>
                {used === 0 ? (
                  <p className="text-sm text-muted-foreground">Empty.</p>
                ) : (
                  <SlotGrid
                    container={container}
                    compact={grid === "compact"}
                    filters={filters}
                    onOpen={(slot) => onOpen(slot, label)}
                  />
                )}
              </div>
            );
          })}

          {items === 0 && <p className="text-sm text-muted-foreground">This player is carrying nothing.</p>}
        </div>
      )}
    </section>
  );
}

// Matches the pals page: saves only change on the game's autosave cycle, so
// polling faster than this mostly re-parses a world nobody has changed.
const REFRESH_OPTIONS = [1, 2, 5, 10];
const DEFAULT_REFRESH_MINUTES = 5;

export function ServerInventory() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [refreshMinutes, setRefreshMinutes] = useState(DEFAULT_REFRESH_MINUTES);
  const [selected, setSelected] = useState<{ slot: ItemSlot; location: string } | null>(null);

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  const infoQuery = useQuery({ queryKey: ["server-info", id], queryFn: () => api.serverInfo(id), retry: false });
  const inventoryQuery = useQuery({
    queryKey: ["server-inventory", id],
    queryFn: () => api.serverInventory(id),
    retry: false,
    refetchInterval: refreshMinutes * 60_000,
    // Same caching as the pals page: re-parsing a large save takes tens of
    // seconds, so leaving the page and coming back shouldn't pay for it again.
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });

  const players = inventoryQuery.data?.players ?? [];
  const filters: Filters = useMemo(() => ({ query, category }), [query, category]);
  const active = filtersActive(filters);
  const statScale = useMemo(() => coreStatScale(players), [players]);

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const notConfigured =
    inventoryQuery.isError && inventoryQuery.error instanceof ApiError && inventoryQuery.error.status === 400;
  const hasData = inventoryQuery.data !== undefined;

  return (
    <div>
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Inventory</h1>
          <p className="mt-0.5 text-sm text-ink/50">
            {serverQuery.data.name} · what each player is carrying, from the save file
          </p>
        </div>
        {inventoryQuery.data && (
          <p className="font-mono text-xs text-ink/40">
            save written {agoLabel(inventoryQuery.data.saveModTime)} · parsed{" "}
            {agoLabel(inventoryQuery.data.parsedAt)}
          </p>
        )}
      </header>

      <div className="space-y-4 p-4 lg:space-y-6 lg:p-8">
        {!hasData && inventoryQuery.isFetching && <SaveReadProgress />}

        {notConfigured && !hasData && <SavePathSetup />}

        {!hasData &&
          inventoryQuery.isError &&
          !notConfigured &&
          (infoQuery.isError ? (
            <ServerUnreachable />
          ) : (
            <p className="text-sm text-destructive">
              Could not read the save file: {(inventoryQuery.error as Error).message}
            </p>
          ))}

        {hasData && inventoryQuery.isFetching && <SaveUpdatingBanner />}

        {hasData && players.length > 0 && (
          <div className="flex flex-wrap items-center gap-3">
            {/* Full width on a phone: sharing a row with the category select
                and a "Clear filters" link squeezed it down to two characters. */}
            <div className="relative w-full min-w-0 sm:w-auto sm:max-w-xs sm:flex-1">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink/30" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search items…"
                className="pl-9"
              />
            </div>

            <label className="flex items-center gap-2">
              <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Category</span>
              <Select value={category} onChange={(e) => setCategory(e.target.value)}>
                <option value="">All</option>
                {CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </Select>
            </label>

            {active && (
              <button
                onClick={() => {
                  setQuery("");
                  setCategory("");
                }}
                className="text-xs font-semibold text-brand-red hover:underline"
              >
                Clear filters
              </button>
            )}

            <div className="ml-auto flex items-center gap-2">
              <label className="flex items-center gap-2">
                <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Refresh</span>
                <Select
                  value={refreshMinutes}
                  onChange={(e) => setRefreshMinutes(Number(e.target.value))}
                  className="font-mono text-xs"
                >
                  {REFRESH_OPTIONS.map((m) => (
                    <option key={m} value={m}>
                      {m} min
                    </option>
                  ))}
                </Select>
              </label>
              <button
                onClick={() => inventoryQuery.refetch()}
                disabled={inventoryQuery.isFetching}
                title="Check for a newer save now"
                aria-label="Refresh now"
                className="rounded-lg border border-ink/15 bg-white p-2 text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink disabled:opacity-50"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", inventoryQuery.isFetching && "animate-spin")} />
              </button>
            </div>
          </div>
        )}

        {hasData &&
          (players.length === 0 ? (
            <p className="text-sm text-muted-foreground">No player inventories in this save yet.</p>
          ) : (
            players.map((player) => (
              <PlayerSection
                key={player.uid}
                player={player}
                open={!collapsed.has(player.uid)}
                onToggle={() =>
                  setCollapsed((prev) => {
                    const next = new Set(prev);
                    next.has(player.uid) ? next.delete(player.uid) : next.add(player.uid);
                    return next;
                  })
                }
                filters={filters}
                statScale={statScale}
                onOpen={(slot, location) => setSelected({ slot, location })}
              />
            ))
          ))}

        {hasData && players.length > 0 && (
          <p className="pt-2 text-xs text-ink/35">
            Item artwork and names © Pocketpair, Inc. Icons and localisation data vendored from{" "}
            <span className="font-mono">palworld-save-pal</span>.
          </p>
        )}
      </div>

      <ItemDetailDialog
        slot={selected?.slot ?? null}
        location={selected?.location ?? ""}
        onClose={() => setSelected(null)}
      />
    </div>
  );
}
