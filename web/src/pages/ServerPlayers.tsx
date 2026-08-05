import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, ChevronDown, RefreshCw, Search } from "lucide-react";
import { api, ApiError, type Pal, type PlayerPals } from "../lib/api";
import { initials, playerColor } from "../lib/palette";
import { agoLabel } from "../lib/time";
import { elementColor, palDeckNo, palDeckSortValue, palEntry, palIconUrl, palName, passiveName, rarityTier } from "../lib/paldex";
import { WORK_TYPES, workLevel } from "../lib/crew";
import { palEffectiveStats } from "../lib/stats";
import { TalentTriplet } from "../components/TalentTriplet";
import { cn } from "../lib/utils";
import { PassiveBadge } from "../components/PassiveBadge";
import { PassiveFilter } from "../components/PassiveFilter";
import { ServerUnreachable } from "../components/ServerUnreachable";
import { SaveReadProgress } from "../components/SaveReadProgress";
import { SaveUpdatingBanner } from "../components/SaveUpdatingBanner";
import { PalDetailDialog } from "../components/PalDetailDialog";
import { WorkIcon, WORK_COLORS } from "../components/WorkIcon";
import { SavePathSetup } from "../components/SavePathSetup";
import { Badge } from "../components/ui/badge";
import { Input } from "../components/ui/input";
import { Select } from "../components/ui/select";

// ---------------------------------------------------------------------------
// Metrics: the numeric dimensions you can sort and filter by. IVs are the raw
// talents; "effective" stats are the calibrated in-game HP/Attack/Defense at
// the pal's level, so a boxed alpha outranks a freshly-caught one.
// ---------------------------------------------------------------------------

type Metric = "iv-total" | "iv-hp" | "iv-atk" | "iv-def" | "eff-hp" | "eff-atk" | "eff-def";
/** `work-<type>` sorts by a work suitability level — the species' own, so
 * every Digtoise ties and the order within a species holds steady. */
type SortKey = "name" | "level" | "deck" | Metric | `work-${string}`;

const WORK_LABELS = Object.fromEntries(WORK_TYPES.map((w) => [w.id, w.label]));

const METRIC_LABELS: Record<Metric, string> = {
  "iv-total": "IV total",
  "iv-hp": "IV HP",
  "iv-atk": "IV Attack",
  "iv-def": "IV Defense",
  "eff-hp": "Effective HP",
  "eff-atk": "Effective Attack",
  "eff-def": "Effective Defense",
};
const METRICS = Object.keys(METRIC_LABELS) as Metric[];

function metricValue(pal: Pal, m: Metric, eff: ReturnType<typeof palEffectiveStats> | undefined): number {
  switch (m) {
    case "iv-total":
      return pal.talentHp + pal.talentShot + pal.talentDefense;
    case "iv-hp":
      return pal.talentHp;
    case "iv-atk":
      return pal.talentShot;
    case "iv-def":
      return pal.talentDefense;
    case "eff-hp":
      return eff?.hp ?? 0;
    case "eff-atk":
      return eff?.attack ?? 0;
    case "eff-def":
      return eff?.defense ?? 0;
  }
}

interface Controls {
  query: string;
  passives: Set<string>;
  minMetric: Metric;
  minValue: number;
  /** Work-suitability filter: a type id, or "" for off. */
  workType: string;
  /** Minimum level for that type; a picked type always implies at least 1. */
  workMin: number;
  sortKey: SortKey;
  sortDir: "asc" | "desc";
  /** instanceId -> effective stats, memoized so sorting doesn't recompute. */
  effMap: Map<string, ReturnType<typeof palEffectiveStats>>;
}

function controlsActive(c: Controls): boolean {
  return Boolean(c.query.trim()) || c.passives.size > 0 || c.minValue > 0 || c.workType !== "";
}

function matchPal(pal: Pal, c: Controls): boolean {
  const q = c.query.trim().toLowerCase();
  if (q) {
    const hit =
      pal.nickname.toLowerCase().includes(q) ||
      pal.characterId.toLowerCase().includes(q) ||
      palName(pal.characterId).toLowerCase().includes(q) ||
      pal.passives.some((p) => passiveName(p).toLowerCase().includes(q) || p.toLowerCase().includes(q));
    if (!hit) return false;
  }
  // Passive filter narrows: a pal must carry every selected passive. Matched
  // by display name, so two internal codes that read the same (e.g. "Brave")
  // count as one filter.
  for (const name of c.passives)
    if (!pal.passives.some((code) => passiveName(code) === name)) return false;
  if (c.minValue > 0 && metricValue(pal, c.minMetric, c.effMap.get(pal.instanceId)) < c.minValue) return false;
  if (c.workType && workLevel(pal.characterId, c.workType) < Math.max(1, c.workMin)) return false;
  return true;
}

function sortPals(pals: Pal[], c: Controls): Pal[] {
  const dir = c.sortDir === "asc" ? 1 : -1;
  return [...pals].sort((a, b) => {
    if (c.sortKey === "name") {
      return dir * palName(a.characterId).localeCompare(palName(b.characterId));
    }
    if (c.sortKey === "deck") {
      return dir * (palDeckSortValue(a.characterId) - palDeckSortValue(b.characterId));
    }
    if (c.sortKey.startsWith("work-")) {
      const type = c.sortKey.slice(5);
      return dir * (workLevel(a.characterId, type) - workLevel(b.characterId, type));
    }
    // The work- branch above leaves only "level" and the metrics, but
    // startsWith doesn't narrow a template-literal type — hence the cast.
    const av = c.sortKey === "level" ? a.level : metricValue(a, c.sortKey as Metric, c.effMap.get(a.instanceId));
    const bv = c.sortKey === "level" ? b.level : metricValue(b, c.sortKey as Metric, c.effMap.get(b.instanceId));
    return dir * (av - bv);
  });
}

// ---------------------------------------------------------------------------

function PalCard({ pal, onOpen, workBadge }: { pal: Pal; onOpen: () => void; workBadge?: string }) {
  const species = palName(pal.characterId);
  const entry = palEntry(pal.characterId);
  const elements = (entry?.elements ?? []).slice(0, 2);
  const tier = rarityTier(entry?.rarity ?? 0);
  // Only while a work sort or filter is on: the number being sorted by has
  // to be visible, or the ordering reads as arbitrary.
  const workBadgeLevel = workBadge ? workLevel(pal.characterId, workBadge) : 0;

  return (
    <button
      onClick={onOpen}
      // content-visibility lets the browser skip laying out and painting cards
      // that are scrolled off-screen — the roster can run to hundreds of pals.
      className="flex w-full gap-3 rounded-xl border border-ink/10 bg-white/70 p-3 text-left transition-colors [contain-intrinsic-size:auto_112px] [content-visibility:auto] hover:border-ink/25 hover:bg-white"
    >
      <div
        className={cn(
          "flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border",
          tier === "legendary"
            ? "border-legendary/40 bg-legendary/10"
            : tier === "rare"
              ? "border-pal-blue/40 bg-pal-blue/10"
              : "border-ink/10 bg-ink/5",
        )}
      >
        <img
          src={palIconUrl(pal.characterId)}
          alt=""
          className="h-10 w-10 object-contain"
          loading="lazy"
          decoding="async"
          // A pal added by a game update has no vendored icon; the frame
          // alone reads fine, so drop the broken image rather than show it.
          onError={(e) => {
            e.currentTarget.style.visibility = "hidden";
          }}
        />
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-2">
          <p className="truncate text-sm font-semibold text-foreground">
            {pal.nickname || species}
            {pal.gender && (
              // Bumped well past the surrounding text: these glyphs draw
              // small for their font size, so at 14px and 70% opacity they
              // were barely visible.
              <span
                className={cn(
                  "ml-1 align-middle text-lg font-bold leading-none",
                  pal.gender === "female" ? "text-brand-red" : "text-pal-blue",
                )}
                title={pal.gender === "female" ? "Female" : "Male"}
                aria-label={pal.gender === "female" ? "Female" : "Male"}
                role="img"
              >
                {pal.gender === "female" ? "♀" : "♂"}
              </span>
            )}
          </p>
          <span className="shrink-0 rounded-full bg-ink px-2 py-0.5 font-mono text-xs font-bold text-paper">
            Lv.{pal.level}
          </span>
        </div>

        <p className="truncate text-xs text-ink/45">
        {palDeckNo(pal.characterId) && <span className="font-mono">#{palDeckNo(pal.characterId)}</span>}
        {pal.nickname ? `${palDeckNo(pal.characterId) ? " · " : ""}${species}` : ""}&nbsp;
      </p>

        <div className="mt-1 flex flex-wrap items-center gap-1">
          {workBadge && workBadgeLevel > 0 && (
            <span
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-bold"
              style={{ backgroundColor: `${WORK_COLORS[workBadge] ?? "#8A8578"}22`, color: WORK_COLORS[workBadge] ?? "#8A8578" }}
            >
              <WorkIcon type={workBadge} className="h-3 w-3" />
              {WORK_LABELS[workBadge] ?? workBadge} {workBadgeLevel}
            </span>
          )}
          {elements.map((el) => (
            <span
              key={el}
              className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
              style={{ backgroundColor: `${elementColor(el)}22`, color: elementColor(el) }}
            >
              {el}
            </span>
          ))}
          {pal.isBoss && (
            <Badge variant="outline" className="border-legendary/40 bg-legendary/10 px-1.5 py-0 text-[10px] text-legendary">
              Alpha
            </Badge>
          )}
          {pal.isLucky && (
            <Badge variant="outline" className="border-brand-amber/40 bg-brand-amber/10 px-1.5 py-0 text-[10px] text-brand-amber">
              Lucky
            </Badge>
          )}
          <TalentTriplet
            hp={pal.talentHp}
            attack={pal.talentShot}
            defense={pal.talentDefense}
            className="font-mono text-[10px] text-ink/40"
          />
        </div>

        {pal.passives.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {pal.passives.map((p) => (
              <PassiveBadge key={p} code={p} />
            ))}
          </div>
        )}
      </div>
    </button>
  );
}

function PalGroup({
  title,
  pals,
  onOpen,
  forceOpen,
  workBadge,
}: {
  title: string;
  pals: Pal[];
  onOpen: (pal: Pal) => void;
  /** While a filter is active every match must be visible, so collapsing is
   * suspended rather than hiding results behind a closed group. */
  forceOpen?: boolean;
  /** Work type whose level each card shows — set while a work sort/filter
   * is on, so the ordering carries its own evidence. */
  workBadge?: string;
}) {
  const [open, setOpen] = useState(true);
  if (pals.length === 0) return null;
  const expanded = forceOpen || open;
  return (
    <div>
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={expanded}
        className={cn(
          "mb-2 flex w-full items-center gap-1 text-left",
          forceOpen && "pointer-events-none",
        )}
      >
        <ChevronDown
          className={cn("h-3.5 w-3.5 text-ink/40 transition-transform", !expanded && "-rotate-90")}
        />
        <p className="text-xs font-semibold uppercase tracking-wide text-ink/40">
          {title} <span className="font-mono text-ink/30">({pals.length})</span>
        </p>
      </button>
      {expanded && (
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {pals.map((pal) => (
            <PalCard key={pal.instanceId} pal={pal} onOpen={() => onOpen(pal)} workBadge={workBadge} />
          ))}
        </div>
      )}
    </div>
  );
}

/** Filtered + sorted pals for a player, split back into their four boxes. */
function partition(player: PlayerPals, c: Controls) {
  const each = (list: Pal[]) => sortPals(list.filter((p) => matchPal(p, c)), c);
  const party = each(player.party);
  const palbox = each(player.palbox);
  const base = each(player.base);
  const storage = each(player.storage ?? []);
  return { party, palbox, base, storage, total: party.length + palbox.length + base.length + storage.length };
}

function PlayerSection({
  player,
  parts,
  filtered,
  open,
  onToggle,
  onOpen,
  workBadge,
}: {
  player: PlayerPals;
  /** This player's filtered/sorted boxes, computed once by the page. */
  parts: ReturnType<typeof partition>;
  /** Whether any filter is active (changes the count label + hides empties). */
  filtered: boolean;
  open: boolean;
  onToggle: () => void;
  onOpen: (pal: Pal, location: string) => void;
  workBadge?: string;
}) {
  const color = playerColor(player.uid);
  const { party, palbox, base, storage, total } = parts;
  const owned =
    player.party.length + player.palbox.length + player.base.length + (player.storage?.length ?? 0);
  const active = filtered;

  // A filter that excludes all of a player's pals hides the player entirely,
  // rather than leaving an empty section to scroll past.
  if (active && total === 0) return null;

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
          <p className="font-mono text-xs text-ink/40">
            Lv.{player.level} · {active ? `${total} of ${owned}` : owned} {owned === 1 && !active ? "pal" : "pals"}
          </p>
        </div>
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-ink/40 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="space-y-5 border-t border-ink/10 p-5">
          <PalGroup title="Party" pals={party} forceOpen={filtered} onOpen={(p) => onOpen(p, "Party")} workBadge={workBadge} />
          <PalGroup title="Palbox" pals={palbox} forceOpen={filtered} onOpen={(p) => onOpen(p, "Palbox")} workBadge={workBadge} />
          <PalGroup title="At base" pals={base} forceOpen={filtered} onOpen={(p) => onOpen(p, "At base")} workBadge={workBadge} />
          <PalGroup
            title="Pal storage"
            pals={storage}
            forceOpen={filtered}
            onOpen={(p) => onOpen(p, "Pal storage")}
            workBadge={workBadge}
          />
          {total === 0 && <p className="text-sm text-muted-foreground">No pals owned yet.</p>}
        </div>
      )}
    </section>
  );
}

/** Multi-select of the passives present in the save, as a dropdown of
 * checkboxes with per-passive counts. Selecting narrows to pals that carry
 * every checked passive. */
// Re-reading is only worth doing about as often as the data can change, and
// party/palbox contents move on human timescales. The game also rewrites
// Level.sav on every autosave (default: every 30s), so a short interval
// means almost every poll misses the mtime cache and re-parses the world.
const REFRESH_OPTIONS = [1, 2, 5, 10];
const DEFAULT_REFRESH_MINUTES = 5;

export function ServerPlayers() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const [query, setQuery] = useState("");
  const [passives, setPassives] = useState<Set<string>>(new Set());
  const [minMetric, setMinMetric] = useState<Metric>("iv-total");
  const [minValue, setMinValue] = useState(0);
  const [workType, setWorkType] = useState("");
  const [workMin, setWorkMin] = useState(1);
  const [sortKey, setSortKey] = useState<SortKey>("level");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [refreshMinutes, setRefreshMinutes] = useState(DEFAULT_REFRESH_MINUTES);
  const [selected, setSelected] = useState<{ pal: Pal; location: string } | null>(null);
  const openPal = (pal: Pal, location: string) => setSelected({ pal, location });

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  const infoQuery = useQuery({ queryKey: ["server-info", id], queryFn: () => api.serverInfo(id), retry: false });
  const palsQuery = useQuery({
    queryKey: ["server-pals", id],
    queryFn: () => api.serverPals(id),
    retry: false,
    refetchInterval: refreshMinutes * 60_000,
    // Keep the parsed result in memory across navigation. Re-parsing a large
    // save takes 20-30s, so the default 5-minute gcTime meant leaving the
    // page and coming back dropped everything and made you wait again.
    gcTime: 60 * 60_000,
    // A remount within the window reuses the cache instead of refetching,
    // so switching tabs and back is instant; the interval still refreshes it.
    staleTime: 60_000,
  });

  const players = palsQuery.data?.players ?? [];

  // Effective stats and passive counts, recomputed only when the roster does.
  const effMap = useMemo(() => {
    const m = new Map<string, ReturnType<typeof palEffectiveStats>>();
    for (const player of players)
      for (const pal of [...player.party, ...player.palbox, ...player.base, ...(player.storage ?? [])])
        m.set(pal.instanceId, palEffectiveStats(pal));
    return m;
  }, [players]);
  // Keyed by display name (so duplicate-named codes merge), counting how many
  // pals carry each passive.
  const passiveCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const player of players)
      for (const pal of [...player.party, ...player.palbox, ...player.base, ...(player.storage ?? [])])
        for (const name of new Set(pal.passives.map(passiveName))) m.set(name, (m.get(name) ?? 0) + 1);
    return m;
  }, [players]);

  // Stable across unrelated re-renders, so the partition memo below only
  // recomputes when a filter/sort input or the roster actually changes.
  const controls: Controls = useMemo(
    () => ({ query, passives, minMetric, minValue, workType, workMin, sortKey, sortDir, effMap }),
    [query, passives, minMetric, minValue, workType, workMin, sortKey, sortDir, effMap],
  );
  const active = controlsActive(controls);
  // The work type in play, from either control — it puts a level badge on
  // every card so a work sort/filter shows the number it ordered by.
  const workBadge = workType || (sortKey.startsWith("work-") ? sortKey.slice(5) : "");

  // Every player's filtered/sorted boxes in one pass — the sections render
  // from this instead of re-partitioning per player per render.
  const partitions = useMemo(() => {
    const m = new Map<string, ReturnType<typeof partition>>();
    for (const p of players) m.set(p.uid, partition(p, controls));
    return m;
  }, [players, controls]);

  // Players with at least one matching pal (all of them when no filter is on).
  const visible = players.filter((p) => !active || (partitions.get(p.uid)?.total ?? 0) > 0);

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const notConfigured =
    palsQuery.isError && palsQuery.error instanceof ApiError && palsQuery.error.status === 400;
  const hasData = palsQuery.data !== undefined;

  const onSortKeyChange = (key: SortKey) => {
    setSortKey(key);
    // Names read best A→Z; everything else you almost always want highest-first.
    setSortDir(key === "name" ? "asc" : "desc");
  };
  const clearFilters = () => {
    setQuery("");
    setPassives(new Set());
    setMinValue(0);
    setWorkType("");
    setWorkMin(1);
  };

  return (
    <div>
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Player pals</h1>
          <p className="mt-0.5 text-sm text-ink/50">
            {serverQuery.data.name} · party &amp; palbox from the save file
          </p>
        </div>
        {palsQuery.data && (
          <p className="font-mono text-xs text-ink/40">
            save written {agoLabel(palsQuery.data.saveModTime)} · parsed {agoLabel(palsQuery.data.parsedAt)}
          </p>
        )}
      </header>

      <div className="space-y-4 p-4 lg:space-y-6 lg:p-8">
        {/* Full progress only on the very first parse; after that a refresh
            shows the banner over the last result instead of blanking. */}
        {!hasData && palsQuery.isFetching && <SaveReadProgress />}

        {notConfigured && !hasData && <SavePathSetup />}

        {!hasData && palsQuery.isError && !notConfigured && (
          infoQuery.isError ? <ServerUnreachable /> : (
            <p className="text-sm text-destructive">Could not read the save file: {(palsQuery.error as Error).message}</p>
          )
        )}

        {hasData && palsQuery.isFetching && <SaveUpdatingBanner />}

        {hasData && players.length > 0 && (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <div className="relative min-w-0 flex-1 sm:max-w-xs">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink/30" />
                <Input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search name, species or passive…"
                  className="pl-9"
                />
              </div>

              <label className="flex items-center gap-2">
                <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Sort</span>
                <Select value={sortKey} onChange={(e) => onSortKeyChange(e.target.value as SortKey)}>
                  <option value="name">Name</option>
                  <option value="level">Level</option>
                  <option value="deck">Paldeck #</option>
                  <optgroup label="Talent (IV)">
                    <option value="iv-total">IV total</option>
                    <option value="iv-hp">IV HP</option>
                    <option value="iv-atk">IV Attack</option>
                    <option value="iv-def">IV Defense</option>
                  </optgroup>
                  <optgroup label="Effective stat">
                    <option value="eff-hp">Effective HP</option>
                    <option value="eff-atk">Effective Attack</option>
                    <option value="eff-def">Effective Defense</option>
                  </optgroup>
                  <optgroup label="Work suitability">
                    {WORK_TYPES.map((w) => (
                      <option key={w.id} value={`work-${w.id}`}>
                        {w.label}
                      </option>
                    ))}
                  </optgroup>
                </Select>
                <button
                  onClick={() => setSortDir((d) => (d === "asc" ? "desc" : "asc"))}
                  title={sortDir === "asc" ? "Ascending" : "Descending"}
                  aria-label={`Sort ${sortDir === "asc" ? "ascending" : "descending"}`}
                  className="rounded-lg border border-ink/15 bg-white p-2 text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink"
                >
                  {sortDir === "asc" ? <ArrowUp className="h-3.5 w-3.5" /> : <ArrowDown className="h-3.5 w-3.5" />}
                </button>
              </label>

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
                {/* Refetches rather than forcing a re-parse: the server reuses
                    its cached read while Level.sav is unchanged. */}
                <button
                  onClick={() => palsQuery.refetch()}
                  disabled={palsQuery.isFetching}
                  title="Check for a newer save now"
                  aria-label="Refresh now"
                  className="rounded-lg border border-ink/15 bg-white p-2 text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink disabled:opacity-50"
                >
                  <RefreshCw className={cn("h-3.5 w-3.5", palsQuery.isFetching && "animate-spin")} />
                </button>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-3">
              <PassiveFilter counts={passiveCounts} selected={passives} onChange={setPassives} />

              <label className="flex items-center gap-1.5">
                <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Min</span>
                <Select value={minMetric} onChange={(e) => setMinMetric(e.target.value as Metric)}>
                  {METRICS.map((m) => (
                    <option key={m} value={m}>
                      {METRIC_LABELS[m]}
                    </option>
                  ))}
                </Select>
                <Input
                  type="number"
                  min={0}
                  value={minValue || ""}
                  placeholder="0"
                  onChange={(e) => setMinValue(Math.max(0, Number(e.target.value) || 0))}
                  className="w-24 text-right"
                />
              </label>

              <label className="flex items-center gap-1.5">
                <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Work</span>
                <Select value={workType} onChange={(e) => setWorkType(e.target.value)}>
                  <option value="">Any work</option>
                  {WORK_TYPES.map((w) => (
                    <option key={w.id} value={w.id}>
                      {w.label}
                    </option>
                  ))}
                </Select>
                {workType && (
                  <Input
                    type="number"
                    min={1}
                    // Species tables already run to 8 post-Yakushima; 10 is
                    // the game's enhanced ceiling, kept for headroom.
                    max={10}
                    value={workMin}
                    aria-label="Minimum work level"
                    onChange={(e) => setWorkMin(Math.max(1, Number(e.target.value) || 1))}
                    className="w-16 text-right"
                  />
                )}
              </label>

              <div className="ml-auto flex items-center gap-2 text-xs">
                {active && (
                  <button onClick={clearFilters} className="font-semibold text-brand-red hover:underline">
                    Clear filters
                  </button>
                )}
                <button
                  onClick={() => setCollapsed(new Set())}
                  className="rounded-lg border border-ink/15 bg-white px-2.5 py-1.5 font-medium text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink"
                >
                  Expand all
                </button>
                <button
                  onClick={() => setCollapsed(new Set(visible.map((p) => p.uid)))}
                  className="rounded-lg border border-ink/15 bg-white px-2.5 py-1.5 font-medium text-ink/60 transition-colors hover:bg-ink/5 hover:text-ink"
                >
                  Collapse all
                </button>
              </div>
            </div>
          </div>
        )}

        {hasData &&
          (players.length === 0 ? (
            <p className="text-sm text-muted-foreground">No players found in this save yet.</p>
          ) : visible.length === 0 ? (
            <p className="text-sm text-muted-foreground">No pals match these filters.</p>
          ) : (
            visible.map((player) => (
              <PlayerSection
                key={player.uid}
                player={player}
                parts={partitions.get(player.uid)!}
                filtered={active}
                open={!collapsed.has(player.uid)}
                onToggle={() =>
                  setCollapsed((prev) => {
                    const next = new Set(prev);
                    next.has(player.uid) ? next.delete(player.uid) : next.add(player.uid);
                    return next;
                  })
                }
                onOpen={openPal}
                workBadge={workBadge}
              />
            ))
          ))}

        {hasData && players.length > 0 && (
          <p className="pt-2 text-xs text-ink/35">
            Pal artwork and names © Pocketpair, Inc. Icons and localisation data vendored from{" "}
            <span className="font-mono">palworld-server-manager</span> and{" "}
            <span className="font-mono">palworld-save-pal</span>.
          </p>
        )}
      </div>

      <PalDetailDialog
        pal={selected?.pal ?? null}
        location={selected?.location ?? ""}
        onClose={() => setSelected(null)}
      />
    </div>
  );
}
