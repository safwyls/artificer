import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Check, ChevronDown, RefreshCw, Search, SlidersHorizontal } from "lucide-react";
import { api, ApiError, type Pal, type PlayerPals } from "../lib/api";
import { initials, playerColor } from "../lib/palette";
import { elementColor, palEntry, palIconUrl, palName, passiveName, rarityTier } from "../lib/paldex";
import { computeStats } from "../lib/stats";
import { cn } from "../lib/utils";
import { ServerUnreachable } from "../components/ServerUnreachable";
import { SaveReadProgress } from "../components/SaveReadProgress";
import { SaveUpdatingBanner } from "../components/SaveUpdatingBanner";
import { PalDetailDialog } from "../components/PalDetailDialog";
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
type SortKey = "name" | "level" | Metric;

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

/** A pal's calibrated in-game stats, or null when the species has no combat
 * data. Level, condenser (rank), souls, passives and alpha all feed in. */
function effectiveOf(pal: Pal) {
  return computeStats({
    characterId: pal.characterId,
    level: pal.level,
    ivHp: pal.talentHp,
    ivAttack: pal.talentShot,
    ivDefense: pal.talentDefense,
    condenser: Math.max(0, pal.rank - 1),
    soulHp: pal.souls["Max HP"] ?? 0,
    soulAttack: pal.souls["Attack"] ?? 0,
    soulDefense: pal.souls["Defense"] ?? 0,
    passives: pal.passives,
    isAlpha: pal.isBoss,
  });
}

function metricValue(pal: Pal, m: Metric, eff: ReturnType<typeof effectiveOf> | undefined): number {
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
  sortKey: SortKey;
  sortDir: "asc" | "desc";
  /** instanceId -> effective stats, memoized so sorting doesn't recompute. */
  effMap: Map<string, ReturnType<typeof effectiveOf>>;
}

function controlsActive(c: Controls): boolean {
  return Boolean(c.query.trim()) || c.passives.size > 0 || c.minValue > 0;
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
  return true;
}

function sortPals(pals: Pal[], c: Controls): Pal[] {
  const dir = c.sortDir === "asc" ? 1 : -1;
  return [...pals].sort((a, b) => {
    if (c.sortKey === "name") {
      return dir * palName(a.characterId).localeCompare(palName(b.characterId));
    }
    const av = c.sortKey === "level" ? a.level : metricValue(a, c.sortKey, c.effMap.get(a.instanceId));
    const bv = c.sortKey === "level" ? b.level : metricValue(b, c.sortKey, c.effMap.get(b.instanceId));
    return dir * (av - bv);
  });
}

// ---------------------------------------------------------------------------

function PalCard({ pal, onOpen }: { pal: Pal; onOpen: () => void }) {
  const species = palName(pal.characterId);
  const entry = palEntry(pal.characterId);
  const elements = (entry?.elements ?? []).slice(0, 2);
  const tier = rarityTier(entry?.rarity ?? 0);

  return (
    <button
      onClick={onOpen}
      className="flex w-full gap-3 rounded-xl border border-ink/10 bg-white/70 p-3 text-left transition-colors hover:border-ink/25 hover:bg-white"
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

        <p className="truncate text-xs text-ink/45">{pal.nickname ? species : ""}&nbsp;</p>

        <div className="mt-1 flex flex-wrap items-center gap-1">
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
          <span className="font-mono text-[10px] text-ink/40" title="IVs: HP / Attack / Defense">
            {pal.talentHp}/{pal.talentShot}/{pal.talentDefense}
          </span>
        </div>

        {pal.passives.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1">
            {pal.passives.map((p) => (
              <span
                key={p}
                title={p}
                className="rounded-full bg-ink/5 px-1.5 py-0.5 text-[10px] text-ink/60"
              >
                {passiveName(p)}
              </span>
            ))}
          </div>
        )}
      </div>
    </button>
  );
}

function PalGroup({ title, pals, onOpen }: { title: string; pals: Pal[]; onOpen: (pal: Pal) => void }) {
  if (pals.length === 0) return null;
  return (
    <div>
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">
        {title} <span className="font-mono text-ink/30">({pals.length})</span>
      </p>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {pals.map((pal) => (
          <PalCard key={pal.instanceId} pal={pal} onOpen={() => onOpen(pal)} />
        ))}
      </div>
    </div>
  );
}

/** Filtered + sorted pals for a player, split back into their three boxes. */
function partition(player: PlayerPals, c: Controls) {
  const each = (list: Pal[]) => sortPals(list.filter((p) => matchPal(p, c)), c);
  const party = each(player.party);
  const palbox = each(player.palbox);
  const base = each(player.base);
  return { party, palbox, base, total: party.length + palbox.length + base.length };
}

function PlayerSection({
  player,
  controls,
  open,
  onToggle,
  onOpen,
}: {
  player: PlayerPals;
  controls: Controls;
  open: boolean;
  onToggle: () => void;
  onOpen: (pal: Pal, location: string) => void;
}) {
  const color = playerColor(player.uid);
  const { party, palbox, base, total } = useMemo(() => partition(player, controls), [player, controls]);
  const owned = player.party.length + player.palbox.length + player.base.length;
  const active = controlsActive(controls);

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
          <PalGroup title="Party" pals={party} onOpen={(p) => onOpen(p, "Party")} />
          <PalGroup title="Palbox" pals={palbox} onOpen={(p) => onOpen(p, "Palbox")} />
          <PalGroup title="At base" pals={base} onOpen={(p) => onOpen(p, "At base")} />
          {total === 0 && <p className="text-sm text-muted-foreground">No pals owned yet.</p>}
        </div>
      )}
    </section>
  );
}

/** Multi-select of the passives present in the save, as a dropdown of
 * checkboxes with per-passive counts. Selecting narrows to pals that carry
 * every checked passive. */
function PassiveFilter({
  counts,
  selected,
  onChange,
}: {
  counts: Map<string, number>;
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
}) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  // Keys are display names, so same-named codes collapse into one row.
  const options = useMemo(
    () =>
      [...counts.keys()]
        .sort((a, b) => a.localeCompare(b))
        .filter((name) => !q.trim() || name.toLowerCase().includes(q.trim().toLowerCase())),
    [counts, q],
  );

  const toggle = (name: string) => {
    const next = new Set(selected);
    next.has(name) ? next.delete(name) : next.add(name);
    onChange(next);
  };

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className={cn(
          "flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors",
          selected.size
            ? "border-brand-red/40 bg-brand-red/10 text-brand-red"
            : "border-ink/15 bg-white text-ink/70 hover:bg-ink/5",
        )}
      >
        <SlidersHorizontal className="h-3.5 w-3.5" />
        Passives
        {selected.size > 0 && <span className="font-mono text-xs">· {selected.size}</span>}
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div className="absolute left-0 top-10 z-30 w-64 overflow-hidden rounded-xl border border-ink/10 bg-paper shadow-lg">
            <div className="border-b border-ink/10 p-2">
              <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Find a passive…" className="h-8 text-sm" />
            </div>
            <div className="max-h-72 overflow-y-auto p-1">
              {options.map((name) => {
                const on = selected.has(name);
                return (
                  <button
                    key={name}
                    onClick={() => toggle(name)}
                    className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-ink/5"
                  >
                    <span
                      className={cn(
                        "flex h-4 w-4 shrink-0 items-center justify-center rounded border",
                        on ? "border-brand-red bg-brand-red text-paper" : "border-ink/25",
                      )}
                    >
                      {on && <Check className="h-3 w-3" />}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-foreground">{name}</span>
                    <span className="shrink-0 font-mono text-[11px] text-ink/40">{counts.get(name)}</span>
                  </button>
                );
              })}
              {options.length === 0 && <p className="px-2 py-3 text-sm text-muted-foreground">No passives match.</p>}
            </div>
            {selected.size > 0 && (
              <button
                onClick={() => onChange(new Set())}
                className="w-full border-t border-ink/10 px-3 py-2 text-left text-xs font-semibold text-brand-red hover:bg-ink/5"
              >
                Clear {selected.size} selected
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function agoLabel(iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  return `${Math.floor(s / 3600)}h ago`;
}

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
    const m = new Map<string, ReturnType<typeof effectiveOf>>();
    for (const player of players)
      for (const pal of [...player.party, ...player.palbox, ...player.base]) m.set(pal.instanceId, effectiveOf(pal));
    return m;
  }, [players]);
  // Keyed by display name (so duplicate-named codes merge), counting how many
  // pals carry each passive.
  const passiveCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const player of players)
      for (const pal of [...player.party, ...player.palbox, ...player.base])
        for (const name of new Set(pal.passives.map(passiveName))) m.set(name, (m.get(name) ?? 0) + 1);
    return m;
  }, [players]);

  const controls: Controls = { query, passives, minMetric, minValue, sortKey, sortDir, effMap };
  const active = controlsActive(controls);

  // Players with at least one matching pal (all of them when no filter is on).
  const visible = players.filter((p) => !active || partition(p, controls).total > 0);

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
                controls={controls}
                open={!collapsed.has(player.uid)}
                onToggle={() =>
                  setCollapsed((prev) => {
                    const next = new Set(prev);
                    next.has(player.uid) ? next.delete(player.uid) : next.add(player.uid);
                    return next;
                  })
                }
                onOpen={openPal}
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
