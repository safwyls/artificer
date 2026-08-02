import type { AchievementsPlayer } from "../lib/api";
import { type Boss, type FightStats, bossLabel, isLaboratory } from "../lib/achievements";
import { elementCounters, palEntry, palIconUrl } from "../lib/paldex";
import { ElementTag } from "./ElementIcon";
import { cn } from "../lib/utils";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/**
 * What one boss fight is, and who on the server has done it.
 *
 * Levels and HP are vendored (see docs/vendored-game-data.md). Elements come
 * from the pal catalog the rest of the app already reads, and the counters are
 * derived from those rather than stored — an element chart is a rule, and a
 * rule copied into thirteen rows is thirteen chances to be wrong.
 */
/**
 * One difficulty of a fight, as a full-width row.
 *
 * Stacked rather than sat side by side because the labels are the problem: a
 * difficulty can be called "Hard · Blightstar Calamity", and three of those in
 * a row wrap into unreadable columns at any width the dialog actually gets.
 * One per line gives the label the room it needs and lets the figures line up
 * down the right edge, which is where you compare them anyway.
 */
function Figure({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div
      className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 rounded-lg border border-ink/10 bg-ink/[0.03] px-3 py-2"
      title={hint}
    >
      <span className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">{label}</span>
      <span className="font-mono text-sm font-bold tabular-nums text-ink">{value}</span>
    </div>
  );
}

function ElementPips({ elements }: { elements: string[] }) {
  return (
    <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
      {elements.map((el) => (
        <ElementTag key={el} element={el} className="text-sm" />
      ))}
    </span>
  );
}

export function BossFightDialog({
  boss,
  fight,
  players,
  kind,
  onClose,
}: {
  boss: Boss | null;
  fight: FightStats | undefined;
  /** Everyone on the server, to say who has and hasn't done this fight. */
  players: AchievementsPlayer[];
  /** Tower-map fights are flags; raids carry a per-player clear count. */
  kind: "tower" | "raid";
  onClose: () => void;
}) {
  if (!boss) return null;

  const lab = isLaboratory(boss);
  const elements = boss.palId ? (palEntry(boss.palId)?.elements ?? []) : [];
  const counters = elementCounters(elements);
  const cleared =
    kind === "raid"
      ? players.filter((p) => (p.records.raids[boss.key] ?? 0) > 0)
      : players.filter((p) => p.records.towers.includes(boss.key));

  const tiers: [string, [number, number] | undefined][] = [
    ["Normal", fight?.normal],
    [fight?.hardTitle ? `Hard · ${fight.hardTitle}` : "Hard", fight?.hard],
    ["Ultra", fight?.ultra],
  ];

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <span
              className={cn(
                "flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-full border-2 border-ink/10",
                lab ? "bg-[#1b0f0c]" : "bg-paper",
              )}
            >
              <img
                src={palIconUrl(boss.palId ?? "")}
                alt=""
                className={cn(
                  "h-full w-full object-contain",
                  lab &&
                    "scale-[0.72] [filter:brightness(0)_drop-shadow(1px_0_0_#E8491D)_drop-shadow(-1px_0_0_#E8491D)_drop-shadow(0_1px_0_#E8491D)_drop-shadow(0_-1px_0_#E8491D)]",
                )}
              />
            </span>
            <div className="min-w-0">
              <DialogTitle className="font-display text-lg font-extrabold">{bossLabel(boss)}</DialogTitle>
              {fight?.title && <p className="text-sm text-ink/55">{fight.title}</p>}
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-4">
          {fight?.where && <p className="text-sm text-ink/60">{fight.where}</p>}

          {(fight?.normal || fight?.hard || fight?.ultra || fight?.levelRange) && (
            <div className="space-y-2">
              {fight?.levelRange && (
                <Figure label="Level" value={`${fight.levelRange[0]}–${fight.levelRange[1]}`} />
              )}
              {tiers.map(([label, tier]) =>
                tier ? (
                  <Figure
                    key={label}
                    label={label}
                    value={`Lv ${tier[0]} · ${tier[1].toLocaleString()} HP`}
                    hint={`${tier[1].toLocaleString()} HP at level ${tier[0]}`}
                  />
                ) : null,
              )}
            </div>
          )}

          {/* No element line for the Laboratory. It borrows Grizzbolt's
              portrait, but the fight is eight different pals, so printing
              "Electric, weak to Ground" would be a confident wrong answer
              about seven of them. The wave list below is the real answer. */}
          {lab ? (
            <p className="text-sm">
              <span className="text-ink/45">Element </span>
              <span className="text-ink/70">Eight pals, eight matchups — bring a spread</span>
            </p>
          ) : (
            <div className="space-y-1.5 text-sm">
              <p>
                <span className="text-ink/45">Element </span>
                {elements.length > 0 ? (
                  <ElementPips elements={elements} />
                ) : (
                  <span className="text-ink/70">Typeless</span>
                )}
              </p>
              <p>
                <span className="text-ink/45">Weak to </span>
                {counters.length > 0 ? (
                  <ElementPips elements={counters} />
                ) : (
                  // Not "unknown" — a typeless boss genuinely has no counter,
                  // and saying so is the useful thing to say.
                  <span className="text-ink/70">Nothing — no element counters it</span>
                )}
              </p>
            </div>
          )}

          {fight?.waves && (
            <div>
              <p className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-ink/35">
                Waves
                <span className="ml-1 normal-case tracking-normal text-ink/30">(modified tower boss pals)</span>
              </p>
              <ol className="space-y-1 text-sm text-ink/70">
                {fight.waves.map((wave, i) => (
                  <li key={wave.join()} className="flex gap-2">
                    <span className="font-mono text-xs text-ink/35">{i + 1}</span>
                    {wave.join(" + ")}
                  </li>
                ))}
              </ol>
            </div>
          )}

          {fight?.note && <p className="text-sm leading-relaxed text-ink/60">{fight.note}</p>}

          {!fight && (
            <p className="text-sm text-ink/60">
              No fight details are vendored for this one yet — see docs/vendored-game-data.md.
            </p>
          )}

          <div className="border-t border-ink/10 pt-3 text-sm">
            <p className="text-ink/45">On this server</p>
            <p className="mt-1 text-ink/75">
              {players.length === 0
                ? "No players in the save yet."
                : cleared.length === 0
                  ? "Nobody has beaten this yet."
                  : `${cleared.map((p) => p.nickname || "Unnamed").join(", ")} ${
                      cleared.length === players.length
                        ? "— everyone."
                        : `— ${cleared.length} of ${players.length}.`
                    }`}
            </p>
            {kind === "raid" && cleared.length > 0 && (
              <p className="mt-1 font-mono text-xs text-ink/45">
                {cleared.map((p) => `${p.nickname} ×${p.records.raids[boss.key]}`).join(" · ")}
              </p>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
