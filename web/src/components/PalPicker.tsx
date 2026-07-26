import { useMemo, useState } from "react";
import { ChevronDown, Search } from "lucide-react";
import type { Pal } from "../lib/api";
import { palName } from "../lib/paldex";
import { BREEDABLE } from "../lib/breeding";
import { cn } from "../lib/utils";
import { PalPortrait } from "./PalPortrait";
import { TalentTriplet } from "./TalentTriplet";
import { Input } from "./ui/input";
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

export interface PickedPal {
  characterId: string;
  /** Present only when chosen from the save, carrying its real stats. */
  save?: SavePal;
}

type Mode = "all" | "save";

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
  // Explicit open/closed choices per player group. Anything untouched
  // falls back to the default — open while searching (matches should be
  // visible), collapsed when browsing a multi-player save — but a tap on
  // the header always wins, so a noisy player can be folded away even
  // mid-search.
  const [groupOverrides, setGroupOverrides] = useState<Map<string, boolean>>(() => new Map());
  const q = query.trim().toLowerCase();

  const species = useMemo(
    () => (q ? BREEDABLE.filter((p) => p.name.toLowerCase().includes(q)) : BREEDABLE),
    [q],
  );
  // The save tab groups by owner so a player can jump straight to their own
  // box; groups collapse when the server has more than one player.
  const groups = useMemo(() => {
    const byUid = new Map<string, { key: string; name: string; pals: SavePal[] }>();
    for (const p of savePals ?? []) {
      const key = p.playerUid || p.playerName;
      let g = byUid.get(key);
      if (!g) byUid.set(key, (g = { key, name: p.playerName || "Unknown player", pals: [] }));
      g.pals.push(p);
    }
    return [...byUid.values()].sort((a, b) => a.name.localeCompare(b.name));
  }, [savePals]);
  const shownGroups = useMemo(() => {
    if (!q) return groups;
    return groups
      .map((g) => ({
        ...g,
        pals: g.pals.filter(
          (p) => p.nickname.toLowerCase().includes(q) || palName(p.characterId).toLowerCase().includes(q),
        ),
      }))
      .filter((g) => g.pals.length > 0);
  }, [groups, q]);
  const groupOpen = (key: string) => groupOverrides.get(key) ?? (q !== "" || groups.length === 1);
  const toggleGroup = (key: string) =>
    setGroupOverrides((prev) => {
      const next = new Map(prev);
      next.set(key, !groupOpen(key));
      return next;
    });

  const pick = (pick: PickedPal) => {
    onPick(pick);
    onOpenChange(false);
    setQuery("");
  };

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
            placeholder={mode === "save" ? "Search your pals…" : "Search species…"}
            className="pl-8"
          />
        </div>

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
            shownGroups.map((g) => (
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
                {groupOpen(g.key) &&
                  g.pals.map((p) => (
                    <button
                      key={p.key}
                      onClick={() => pick({ characterId: p.characterId, save: p })}
                      className="flex w-full items-center gap-3 rounded-xl border border-transparent p-1.5 text-left hover:border-ink/10 hover:bg-white"
                    >
                      <PalPortrait characterId={p.characterId} size="sm" />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-semibold text-foreground">
                          {p.nickname || palName(p.characterId)}
                          {p.gender && (
                            <span
                              className={cn("ml-1", p.gender === "female" ? "text-brand-red" : "text-pal-blue")}
                              aria-label={p.gender === "female" ? "Female" : "Male"}
                            >
                              {p.gender === "female" ? "♀" : "♂"}
                            </span>
                          )}
                        </p>
                        <p className="truncate font-mono text-[11px] text-ink/40">
                          Lv.{p.level} · {palName(p.characterId)}
                        </p>
                      </div>
                      <TalentTriplet
                        hp={p.ivHp}
                        attack={p.ivAttack}
                        defense={p.ivDefense}
                        className="shrink-0 font-mono text-[11px] text-ink/40"
                      />
                    </button>
                  ))}
              </div>
            ))}
          {mode === "save" && shownGroups.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {saveStatus ?? (q ? `No pal in the save matches “${query}”.` : "No pals found in the save.")}
            </p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
