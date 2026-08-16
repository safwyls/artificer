import { memo, useCallback, useDeferredValue, useEffect, useMemo, useState } from "react";
import { ChevronDown, Search } from "lucide-react";
import type { Pal } from "../lib/api";
import { palName, passiveName } from "../lib/paldex";
import { BREEDABLE } from "../lib/breeding";
import { cn } from "../lib/utils";
import { PalPortrait } from "./PalPortrait";
import { PassiveBadge } from "./PassiveBadge";
import { PassiveFilter } from "./PassiveFilter";
import { TalentTriplet } from "./TalentTriplet";
import { Input } from "./ui/input";
import { Select } from "./ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/** A pal drawn from the server's save, flattened out of the per-player boxes
 * so it can seed a calculator with real level and talents. */
export interface SavePal {
  key: string;
  characterId: string;
  nickname: string;
  level: number;
  gender: "male" | "female" | "";
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
  /** Condenser stars, 0–4. */
  condenser: number;
  souls: { hp: number; attack: number; defense: number };
  passives: string[];
  /** Captured field boss — carries an HP bonus. */
  isAlpha: boolean;
  /** Bond/trust rank 0–10, mapped from the save's friendship points. */
  trust: number;
  playerUid: string;
  playerName: string;
  /** The save's full record and which box it sits in, for the detail dialog. */
  pal: Pal;
  where: string;
}

/** A save pal with its search fields derived once, at load, rather than per
 * keystroke — see the `groups` memo. */
interface IndexedPal {
  pal: SavePal;
  /** Nickname + species + passive display names, lowercased. */
  text: string;
  /** Passive display names, for the exact-match passive filter. */
  names: Set<string>;
}

export interface PickedPal {
  characterId: string;
  /** Present only when chosen from the save, carrying its real stats. */
  save?: SavePal;
}

type Mode = "all" | "save";

/**
 * The talent a "minimum" filter thresholds on. Only the three inheritable
 * talents are offered — a parent's level and soul upgrades don't pass to an
 * egg, so the roster page's effective-stat metrics would be misleading here.
 */
type IvMetric = "total" | "hp" | "attack" | "defense";

const IV_LABELS: Record<IvMetric, string> = {
  total: "IV total",
  hp: "IV HP",
  attack: "IV Attack",
  defense: "IV Defense",
};

/**
 * How many pals a group shows before the reveal row, and how many each tap
 * of it adds. A real save runs to thousands of pals — the fixture this was
 * tuned against holds 6,332 across four players — and every one of them is
 * a portrait, up to four passive chips and a talent triplet. Rendering the
 * lot on a one-character query (which matches nearly everything, and
 * force-opens every group) is what made typing feel stuck. A picker's job
 * is to surface the pal you're after, so the cap costs nothing you'd miss:
 * narrow further, or reveal more.
 */
const GROUP_PAGE = 60;
const GROUP_PAGE_MORE = 200;

function ivValue(p: SavePal, metric: IvMetric): number {
  switch (metric) {
    case "total":
      return p.ivHp + p.ivAttack + p.ivDefense;
    case "hp":
      return p.ivHp;
    case "attack":
      return p.ivAttack;
    case "defense":
      return p.ivDefense;
  }
}

export function PalPicker({
  open,
  onOpenChange,
  onPick,
  title = "Pick a pal",
  savePals,
  saveStatus,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (pick: PickedPal) => void;
  title?: string;
  /** Undefined until the save has been read; empty array means read-but-none. */
  savePals?: SavePal[];
  /** Short line explaining why the save list is empty, when it is. */
  saveStatus?: string;
}) {
  const [mode, setMode] = useState<Mode>("all");
  const [query, setQuery] = useState("");
  // Box-diving filters, save tab only: every selected passive must be on the
  // pal, and its chosen talent must clear the threshold. They outlive a pick
  // on purpose — you're usually hunting both parents to the same spec.
  const [wantPassives, setWantPassives] = useState<Set<string>>(new Set());
  const [ivMetric, setIvMetric] = useState<IvMetric>("total");
  const [ivMin, setIvMin] = useState(0);
  // Explicit open/closed choices per player group. Anything untouched
  // falls back to the default — open while a search or filter is on
  // (matches should be visible), collapsed when browsing a multi-player
  // save — but a tap on the header always wins, so a noisy player can be
  // folded away mid-search.
  const [groupOverrides, setGroupOverrides] = useState<Map<string, boolean>>(() => new Map());
  // How many rows each group has revealed past the cap. Reset whenever the
  // filter changes, so a fresh search always starts at the top of its own
  // matches rather than inheriting a previous hunt's expansion.
  const [revealed, setRevealed] = useState<Map<string, number>>(() => new Map());
  // Filtering thousands of rows is the slow half of a keystroke; letting it
  // lag the input by a frame keeps typing at full speed.
  const q = useDeferredValue(query.trim().toLowerCase());

  const species = useMemo(
    () => (q ? BREEDABLE.filter((p) => p.name.toLowerCase().includes(q)) : BREEDABLE),
    [q],
  );
  // The save tab groups by owner so a player can jump straight to their own
  // box; groups collapse when the server has more than one player. Each pal
  // is indexed once here — its searchable text and its passives' display
  // names — so a keystroke is an includes() per pal instead of re-deriving
  // every name and lowercasing it again.
  const groups = useMemo(() => {
    const byUid = new Map<string, { key: string; name: string; pals: IndexedPal[] }>();
    for (const p of savePals ?? []) {
      const key = p.playerUid || p.playerName;
      let g = byUid.get(key);
      if (!g) byUid.set(key, (g = { key, name: p.playerName || "Unknown player", pals: [] }));
      const names = new Set(p.passives.map(passiveName));
      g.pals.push({
        pal: p,
        text: `${p.nickname} ${palName(p.characterId)} ${[...names].join(" ")}`.toLowerCase(),
        names,
      });
    }
    return [...byUid.values()].sort((a, b) => a.name.localeCompare(b.name));
  }, [savePals]);
  // Display name -> how many pals in the save carry it, for the filter menu.
  const passiveCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const p of savePals ?? [])
      for (const name of new Set(p.passives.map(passiveName))) m.set(name, (m.get(name) ?? 0) + 1);
    return m;
  }, [savePals]);

  /** Any of search, passives or the talent floor is on. */
  const narrowed = q !== "" || wantPassives.size > 0 || ivMin > 0;
  const shownGroups = useMemo(() => {
    if (!narrowed) return groups;
    const match = ({ pal, text, names }: IndexedPal) => {
      if (q && !text.includes(q)) return false;
      // Matched by display name, so two codes that read the same count as one.
      for (const name of wantPassives) if (!names.has(name)) return false;
      if (ivMin > 0 && ivValue(pal, ivMetric) < ivMin) return false;
      return true;
    };
    return groups.map((g) => ({ ...g, pals: g.pals.filter(match) })).filter((g) => g.pals.length > 0);
  }, [groups, q, narrowed, wantPassives, ivMetric, ivMin]);
  const matched = useMemo(() => shownGroups.reduce((n, g) => n + g.pals.length, 0), [shownGroups]);

  useEffect(() => {
    // Returning the same map when it's already empty lets React bail out,
    // so the common "no reveals yet" case costs no extra render.
    setRevealed((prev) => (prev.size === 0 ? prev : new Map()));
  }, [q, wantPassives, ivMetric, ivMin]);

  const groupOpen = (key: string) => groupOverrides.get(key) ?? (narrowed || groups.length === 1);
  const toggleGroup = (key: string) =>
    setGroupOverrides((prev) => {
      const next = new Map(prev);
      next.set(key, !groupOpen(key));
      return next;
    });
  const revealMore = (key: string) =>
    setRevealed((prev) => new Map(prev).set(key, (prev.get(key) ?? GROUP_PAGE) + GROUP_PAGE_MORE));

  // Stable so the memoized rows below aren't invalidated every render.
  const pick = useCallback(
    (picked: PickedPal) => {
      onPick(picked);
      onOpenChange(false);
      setQuery("");
    },
    [onPick, onOpenChange],
  );

  const tabClass = (active: boolean) =>
    cn(
      "flex-1 rounded-lg px-3 py-1.5 text-sm font-semibold transition-colors",
      active ? "bg-white text-ink shadow-sm" : "text-ink/50 hover:text-ink",
    );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>

        {savePals !== undefined && (
          <div className="mt-2 flex gap-1 rounded-xl bg-ink/5 p-1">
            <button className={tabClass(mode === "all")} onClick={() => setMode("all")}>
              All pals
            </button>
            <button className={tabClass(mode === "save")} onClick={() => setMode("save")}>
              From your save
            </button>
          </div>
        )}

        <div className="relative mt-3">
          <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink/30" />
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={mode === "save" ? "Search name, species or passive…" : "Search species…"}
            className="pl-8"
          />
        </div>

        {mode === "save" && savePals && savePals.length > 0 && (
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <PassiveFilter counts={passiveCounts} selected={wantPassives} onChange={setWantPassives} />
            <label className="flex items-center gap-1.5">
              <span className="text-xs font-semibold uppercase tracking-wide text-ink/40">Min</span>
              <Select value={ivMetric} onChange={(e) => setIvMetric(e.target.value as IvMetric)}>
                {(Object.keys(IV_LABELS) as IvMetric[]).map((m) => (
                  <option key={m} value={m}>
                    {IV_LABELS[m]}
                  </option>
                ))}
              </Select>
              <Input
                type="number"
                min={0}
                value={ivMin || ""}
                placeholder="0"
                onChange={(e) => setIvMin(Math.max(0, Number(e.target.value) || 0))}
                className="w-16 text-right font-mono"
                aria-label={`Minimum ${IV_LABELS[ivMetric]}`}
              />
            </label>
            {narrowed && (
              <button
                type="button"
                onClick={() => {
                  setQuery("");
                  setWantPassives(new Set());
                  setIvMin(0);
                }}
                className="ml-auto text-xs font-semibold text-brand-red hover:underline"
              >
                Clear
              </button>
            )}
          </div>
        )}

        {mode === "save" && narrowed && savePals && savePals.length > 0 && (
          <p className="mt-2 font-mono text-[11px] text-ink/40">
            {matched} of {savePals.length} pals
          </p>
        )}

        <div className="mt-3 max-h-[26rem] space-y-1 overflow-y-auto pr-1">
          {mode === "all" &&
            species.map((p) => (
              <button
                key={p.id}
                onClick={() => pick({ characterId: p.id })}
                className="flex w-full items-center gap-3 rounded-xl border border-transparent p-1.5 text-left hover:border-ink/10 hover:bg-white"
              >
                <PalPortrait characterId={p.id} size="sm" />
                <span className="text-sm font-semibold text-foreground">{p.name}</span>
              </button>
            ))}
          {mode === "all" && species.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">No pal matches “{query}”.</p>
          )}

          {mode === "save" &&
            shownGroups.map((g) => {
              const cap = revealed.get(g.key) ?? GROUP_PAGE;
              const hidden = g.pals.length - cap;
              return (
                <div key={g.key}>
                  <button
                    onClick={() => toggleGroup(g.key)}
                    className="sticky top-0 z-10 flex w-full items-center gap-2 border-b border-ink/10 bg-card py-2 pl-1 pr-2 text-left"
                  >
                    <ChevronDown
                      className={cn("h-4 w-4 text-ink/40 transition-transform", !groupOpen(g.key) && "-rotate-90")}
                    />
                    <span className="font-display text-sm font-bold">{g.name}</span>
                    <span className="ml-auto rounded-full bg-ink/5 px-2 py-0.5 font-mono text-[11px] text-ink/50">
                      {g.pals.length}
                    </span>
                  </button>
                  {groupOpen(g.key) && (
                    <>
                      {g.pals.slice(0, cap).map(({ pal }) => (
                        <SavePalRow key={pal.key} pal={pal} onPick={pick} />
                      ))}
                      {hidden > 0 && (
                        <button
                          onClick={() => revealMore(g.key)}
                          className="w-full rounded-xl py-2 text-center font-mono text-[11px] text-ink/40 hover:bg-ink/5 hover:text-ink/60"
                        >
                          Show {Math.min(hidden, GROUP_PAGE_MORE)} more · {hidden} hidden
                        </button>
                      )}
                    </>
                  )}
                </div>
              );
            })}
          {mode === "save" && shownGroups.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {saveStatus ??
                (narrowed ? "No pal in the save matches these filters." : "No pals found in the save.")}
            </p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

/** One pal from the save. Memoized: a keystroke rewrites the whole list, and
 * the rows that survive it are identical — reconciling their portraits and
 * passive chips again is the bulk of the work a search would otherwise do. */
const SavePalRow = memo(function SavePalRow({
  pal,
  onPick,
}: {
  pal: SavePal;
  onPick: (pick: PickedPal) => void;
}) {
  return (
    <button
      onClick={() => onPick({ characterId: pal.characterId, save: pal })}
      className="flex w-full items-start gap-3 rounded-xl border border-transparent p-1.5 text-left hover:border-ink/10 hover:bg-white"
    >
      <PalPortrait characterId={pal.characterId} size="sm" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold text-foreground">
          {pal.nickname || palName(pal.characterId)}
          {pal.gender && (
            <span
              className={cn("ml-1", pal.gender === "female" ? "text-brand-red" : "text-pal-blue")}
              aria-label={pal.gender === "female" ? "Female" : "Male"}
            >
              {pal.gender === "female" ? "♀" : "♂"}
            </span>
          )}
        </p>
        <p className="truncate font-mono text-[11px] text-ink/40">
          Lv.{pal.level} · {palName(pal.characterId)}
        </p>
        {pal.passives.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {pal.passives.map((code, i) => (
              <PassiveBadge key={`${code}-${i}`} code={code} />
            ))}
          </div>
        )}
      </div>
      <TalentTriplet
        hp={pal.ivHp}
        attack={pal.ivAttack}
        defense={pal.ivDefense}
        className="shrink-0 pt-0.5 font-mono text-[11px] text-ink/40"
      />
    </button>
  );
});
