import { type FieldBossPin, fieldBossIconUrl } from "../lib/fieldBosses";
import { elementCounters } from "../lib/elements";
import { ElementTag } from "./ElementIcon";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/**
 * What a field boss on the map is: which pal, what level, and what to bring.
 *
 * Its own dialog rather than BossFightDialog's: that one is about the boss
 * *chain*, and needs vendored level/HP tables and per-player clear state the
 * map doesn't load. Everything here rides on the pin itself — name, level and
 * elements are baked into the field boss table, and the counters come off the
 * element chart — so opening one costs the map no extra data.
 */
export function FieldBossDialog({ boss, onClose }: { boss: FieldBossPin | null; onClose: () => void }) {
  if (!boss) return null;
  const counters = elementCounters(boss.elements);

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <span className="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-full border-2 border-ink/10 bg-paper">
              <img src={fieldBossIconUrl(boss.palId)} alt="" className="h-full w-full object-contain" />
            </span>
            <div className="min-w-0">
              <DialogTitle className="font-display text-lg font-extrabold">{boss.name}</DialogTitle>
              <p className="text-sm text-ink/55">Field boss{boss.level ? ` · Level ${boss.level}` : ""}</p>
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-1.5 text-sm">
          <p>
            <span className="text-ink/45">Element </span>
            {boss.elements.length > 0 ? (
              <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
                {boss.elements.map((el) => (
                  <ElementTag key={el} element={el} />
                ))}
              </span>
            ) : (
              <span className="text-ink/70">Typeless</span>
            )}
          </p>
          <p>
            <span className="text-ink/45">Weak to </span>
            {counters.length > 0 ? (
              <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
                {counters.map((el) => (
                  <ElementTag key={el} element={el} />
                ))}
              </span>
            ) : (
              <span className="text-ink/70">Nothing — no element counters it</span>
            )}
          </p>
          {/* Whether *this* player has beaten it lives on the Achievements
              page, which loads the save records the map doesn't. */}
          <p className="pt-2 text-xs text-ink/45">
            Who has beaten this is on the Achievements page, under Field bosses.
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
