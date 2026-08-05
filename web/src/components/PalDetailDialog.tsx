import { Star } from "lucide-react";
import type { Pal } from "../lib/api";
import {
  elementColor,
  palBaseStats,
  palDeckNo,
  palEntry,
  palIconUrl,
  palName,
  passiveDescription,
  passiveName,
  rarityTier,
  skillDescription,
  skillName,
} from "../lib/paldex";
import { partnerSkill, partnerTags } from "../lib/partner";
import { friendshipRank, palEffectiveStats, talentTone, STAT_COLORS } from "../lib/stats";
import { cn } from "../lib/utils";
import { PassiveTierTile } from "./PassiveBadge";
import { Badge } from "./ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/** A stat with a value and a bar, scaled to a per-stat ceiling so a strong
 * pal fills it and a weak one doesn't. */
function StatBar({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  const pct = Math.min(100, (value / max) * 100);
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-ink/50">{label}</span>
        <span className="font-mono text-xs font-bold" style={{ color }}>
          {value.toLocaleString()}
        </span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-ink/10">
        <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: color }} />
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-ink/10 bg-ink/[0.03] px-3 py-2">
      <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">{label}</p>
      <p className="mt-0.5 font-mono text-sm font-bold text-ink">{value}</p>
    </div>
  );
}

/** In-game position for a container slot: the palbox and pal storage
 * arrange pals in 30-slot pages (6×5), so raw "slot 47" is really page 2,
 * slot 18 — which is where a player will actually find the pal. Party and
 * base containers are single grids and keep a bare slot number. */
const PALBOX_PAGE_SIZE = 30;
export function palPosition(location: string, slotIndex: number): string {
  if (slotIndex < 0) return "";
  if (!/palbox|storage/i.test(location)) return `slot ${slotIndex + 1}`;
  return `page ${Math.floor(slotIndex / PALBOX_PAGE_SIZE) + 1}, slot ${(slotIndex % PALBOX_PAGE_SIZE) + 1}`;
}

export function PalDetailDialog({
  pal,
  location,
  onClose,
}: {
  pal: Pal | null;
  location: string;
  onClose: () => void;
}) {
  if (!pal) return null;

  const species = palName(pal.characterId);
  const entry = palEntry(pal.characterId);
  const partner = partnerSkill(pal.characterId);
  const base = palBaseStats(pal.characterId);
  const tier = rarityTier(entry?.rarity ?? 0);
  const souls = Object.entries(pal.souls ?? {});
  // rank is 1-based (1 = never condensed), so stars run 0–4.
  const stars = Math.max(0, Math.min(4, pal.rank - 1));
  const eff = palEffectiveStats(pal);
  const ivs = [
    ["HP", pal.talentHp],
    ["ATK", pal.talentShot],
    ["DEF", pal.talentDefense],
  ] as const;

  return (
    <Dialog open={pal !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-3 pr-8">
            <span
              className={cn(
                "flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border",
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
                className="h-12 w-12 object-contain"
                onError={(e) => {
                  e.currentTarget.style.visibility = "hidden";
                }}
              />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate">
                {pal.nickname || species}
                {pal.gender && (
                  <span
                    className={cn("ml-1.5 text-xl", pal.gender === "female" ? "text-brand-red" : "text-pal-blue")}
                    aria-label={pal.gender === "female" ? "Female" : "Male"}
                    role="img"
                  >
                    {pal.gender === "female" ? "♀" : "♂"}
                  </span>
                )}
              </span>
              <span className="block text-sm font-normal text-ink/50">
                {palDeckNo(pal.characterId) && (
                  <span className="font-mono">#{palDeckNo(pal.characterId)} · </span>
                )}
                {pal.nickname ? `${species} · ` : ""}Lv.{pal.level} · {location}
                {pal.slotIndex >= 0 && ` · ${palPosition(location, pal.slotIndex)}`}
              </span>
            </span>
            {/* Raw talents, stacked like the game's Potential box. */}
            <span
              className="shrink-0 space-y-0.5 rounded-lg border border-ink/10 bg-ink/[0.03] px-2.5 py-1.5 font-normal"
              title="Talents (IVs): HP / Attack / Defense"
            >
              {ivs.map(([label, val]) => (
                <span key={label} className="flex items-center justify-between gap-3 leading-tight">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">{label}</span>
                  <span className="font-mono text-sm font-bold" style={{ color: talentTone(val) }}>
                    {val}
                  </span>
                </span>
              ))}
            </span>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-1.5">
            {(entry?.elements ?? []).map((el) => (
              <span
                key={el}
                className="rounded px-2 py-0.5 text-xs font-semibold"
                style={{ backgroundColor: `${elementColor(el)}22`, color: elementColor(el) }}
              >
                {el}
              </span>
            ))}
            {pal.isBoss && (
              <Badge variant="outline" className="border-legendary/40 bg-legendary/10 text-legendary">
                Alpha
              </Badge>
            )}
            {pal.isLucky && (
              <Badge variant="outline" className="border-brand-amber/40 bg-brand-amber/10 text-brand-amber">
                Lucky
              </Badge>
            )}
            {/* Condenser rank as the game draws it: four stars, filled as
                condensed — and the empty row is information too, which is
                why it isn't hidden the way the old +n badge was. */}
            <span
              className="inline-flex items-center gap-0.5 rounded-full border border-ink/10 bg-ink/[0.03] px-2 py-1"
              role="img"
              aria-label={`Condenser: ${stars} of 4 stars`}
              title={`Condenser: ${stars} of 4 stars`}
            >
              {Array.from({ length: 4 }, (_, i) => (
                <Star
                  key={i}
                  aria-hidden
                  className={cn("h-3.5 w-3.5", i < stars ? "fill-brand-amber text-brand-amber" : "text-ink/20")}
                />
              ))}
            </span>
          </div>

          {pal.sick && (
            <p className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              Ailing: {pal.sick.replace(/([a-z])([A-Z])/g, "$1 $2")} — a sick pal stops working at a base until
              treated.
            </p>
          )}

          {partner && (
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Partner skill</p>
              <div className="rounded-lg border border-ink/10 bg-white/60 px-3 py-2">
                <p className="flex flex-wrap items-center gap-1.5 text-sm font-semibold text-ink">
                  {partner.n}
                  {partnerTags(partner).map((t) => (
                    <span
                      key={t.label}
                      className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
                      style={{
                        backgroundColor: `${t.bond ? "#F2A93B" : t.element ? elementColor(t.element) : "#5F5850"}22`,
                        color: t.bond ? "#F2A93B" : t.element ? elementColor(t.element) : "#5F5850",
                      }}
                    >
                      {t.label}
                    </span>
                  ))}
                </p>
                {partner.d && <p className="mt-0.5 text-xs text-ink/55">{partner.d}</p>}
              </div>
            </div>
          )}

          {eff && (
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Effective stats</p>
              <div className="grid grid-cols-3 gap-3">
                <StatBar label="HP" value={eff.hp} max={15000} color={STAT_COLORS.hp} />
                <StatBar label="Attack" value={eff.attack} max={1500} color={STAT_COLORS.attack} />
                <StatBar label="Defense" value={eff.defense} max={1500} color={STAT_COLORS.defense} />
              </div>
              <p className="mt-2 text-[10px] text-ink/35">
                Estimated in-game stats at this level. Trust is the one estimate, so a high-bond pal may read a touch
                low.
              </p>
            </div>
          )}

          <div className="grid grid-cols-3 gap-2">
            {/* stomach < 0 is the extractor saying "full" — the save omits
                the field entirely when nothing has been eaten off it. */}
            <Stat
              label="Stomach"
              value={
                pal.stomach < 0
                  ? base?.stomach
                    ? `${base.stomach}/${base.stomach}`
                    : "Full"
                  : base?.stomach
                    ? `${Math.round(pal.stomach)}/${base.stomach}`
                    : String(Math.round(pal.stomach))
              }
            />
            <Stat label="Sanity" value={`${Math.round(pal.sanity)}/100`} />
            <Stat label="Trust" value={`${friendshipRank(pal.friendship)} / 10`} />
          </div>

          {pal.skills.length > 0 && (
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Equipped skills</p>
              <div className="space-y-1.5">
                {pal.skills.map((s) => (
                  <div key={s} className="rounded-lg border border-ink/10 bg-white/60 px-3 py-2">
                    <p className="text-sm font-semibold text-ink">{skillName(s)}</p>
                    {skillDescription(s) && <p className="mt-0.5 text-xs text-ink/55">{skillDescription(s)}</p>}
                  </div>
                ))}
              </div>
            </div>
          )}

          {pal.passives.length > 0 && (
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Passive skills</p>
              <div className="space-y-1.5">
                {pal.passives.map((p) => (
                  <div key={p} className="rounded-lg border border-ink/10 bg-white/60 px-3 py-2">
                    <p className="flex items-center gap-1.5 text-sm font-semibold text-ink">
                      <PassiveTierTile code={p} />
                      {passiveName(p)}
                    </p>
                    {passiveDescription(p) && <p className="mt-0.5 text-xs text-ink/55">{passiveDescription(p)}</p>}
                  </div>
                ))}
              </div>
            </div>
          )}

          {souls.length > 0 && (
            <div>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Soul upgrades</p>
              <div className="flex flex-wrap gap-1.5">
                {souls.map(([stat, points]) => (
                  <span key={stat} className="rounded-full bg-ink/5 px-2 py-1 font-mono text-xs text-ink/60">
                    {stat} +{points}
                  </span>
                ))}
              </div>
            </div>
          )}

          <p className="border-t border-ink/10 pt-3 font-mono text-[10px] text-ink/30">
            {pal.characterId} · {pal.instanceId}
          </p>
        </div>
      </DialogContent>
    </Dialog>
  );
}
