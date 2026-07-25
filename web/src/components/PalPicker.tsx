import { useMemo, useState } from "react";
import { Search } from "lucide-react";
import { palName } from "../lib/paldex";
import { BREEDABLE } from "../lib/breeding";
import { cn } from "../lib/utils";
import { PalPortrait } from "./PalPortrait";
import { Input } from "./ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/** A pal drawn from the server's save, flattened out of the per-player boxes
 * so it can seed a calculator with real level and talents. */
export interface SavePal {
  key: string;
  characterId: string;
  nickname: string;
  level: number;
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
  playerName: string;
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
  const q = query.trim().toLowerCase();

  const species = useMemo(
    () => (q ? BREEDABLE.filter((p) => p.name.toLowerCase().includes(q)) : BREEDABLE),
    [q],
  );
  const saved = useMemo(() => {
    if (!savePals) return [];
    if (!q) return savePals;
    return savePals.filter(
      (p) => p.nickname.toLowerCase().includes(q) || palName(p.characterId).toLowerCase().includes(q),
    );
  }, [savePals, q]);

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
            saved.map((p) => (
              <button
                key={p.key}
                onClick={() => pick({ characterId: p.characterId, save: p })}
                className="flex w-full items-center gap-3 rounded-xl border border-transparent p-1.5 text-left hover:border-ink/10 hover:bg-white"
              >
                <PalPortrait characterId={p.characterId} size="sm" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold text-foreground">
                    {p.nickname || palName(p.characterId)}
                  </p>
                  <p className="truncate font-mono text-[11px] text-ink/40">
                    Lv.{p.level} · {palName(p.characterId)} · {p.playerName}
                  </p>
                </div>
                <span className="shrink-0 font-mono text-[11px] text-ink/40">
                  {p.ivHp}/{p.ivAttack}/{p.ivDefense}
                </span>
              </button>
            ))}
          {mode === "save" && saved.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {saveStatus ?? "No pals found in the save."}
            </p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
