import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Sparkles, X } from "lucide-react";
import { api, ApiError } from "../lib/api";
import { palEntry, palName, passiveName, elementColor } from "../lib/paldex";
import { breedChild, parentPairsFor, isBreedable } from "../lib/breeding";
import { computeStats, talentRating, hasCombatStats, passiveStatEffect, friendshipRank } from "../lib/stats";
import { cn } from "../lib/utils";
import { PalPortrait } from "../components/PalPortrait";
import { PalPicker, type PickedPal, type SavePal } from "../components/PalPicker";
import { NumberField as NumberInput } from "../components/ui/number-field";

type Mode = "breeding" | "stats";
/** Which slot a pending pick lands in; the whole page shares one picker. */
type PickTarget = "a" | "b" | "stats" | "reverse" | null;

export function ServerCalculators() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const [mode, setMode] = useState<Mode>("breeding");

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  // The pal viewer's read is shared, so opening Calculators after Player pals
  // costs no extra parse. retry:false so a save-less server fails fast.
  const palsQuery = useQuery({
    queryKey: ["server-pals", id],
    queryFn: () => api.serverPals(id),
    retry: false,
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });

  const savePals: SavePal[] | undefined = useMemo(() => {
    if (!palsQuery.data) return undefined;
    const out: SavePal[] = [];
    const seen = new Set<string>();
    for (const player of palsQuery.data.players) {
      for (const pal of [...player.party, ...player.palbox, ...player.base]) {
        if (seen.has(pal.instanceId)) continue;
        seen.add(pal.instanceId);
        // Soul upgrades come back keyed by the game's stat labels; pull the
        // three combat ones. Rank is 1-based (1 = no condenser), so stars = rank-1.
        const souls = pal.souls ?? {};
        out.push({
          key: pal.instanceId,
          characterId: pal.characterId,
          nickname: pal.nickname,
          level: pal.level,
          ivHp: pal.talentHp,
          ivAttack: pal.talentShot,
          ivDefense: pal.talentDefense,
          condenser: Math.max(0, (pal.rank ?? 1) - 1),
          souls: {
            hp: souls["Max HP"] ?? 0,
            attack: souls["Attack"] ?? 0,
            defense: souls["Defense"] ?? 0,
          },
          passives: pal.passives ?? [],
          isAlpha: pal.isBoss,
          trust: friendshipRank(pal.friendship),
          playerName: player.nickname,
        });
      }
    }
    return out;
  }, [palsQuery.data]);

  const saveStatus =
    palsQuery.isLoading
      ? "Reading the save…"
      : palsQuery.isError
        ? palsQuery.error instanceof ApiError && palsQuery.error.status === 400
          ? "Add a save path to this server to pick from your own pals."
          : "Couldn't read the save."
        : savePals && savePals.length === 0
          ? "No pals in the save yet."
          : undefined;

  const segClass = (active: boolean) =>
    cn(
      "rounded-lg px-4 py-1.5 text-sm font-bold transition-colors",
      active ? "bg-brand-red text-paper" : "text-ink/60 hover:text-ink",
    );

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading…</p>;
  if (serverQuery.isError || !serverQuery.data)
    return <p className="p-6 text-destructive">Server not found.</p>;

  return (
    <div>
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Calculators</h1>
          <p className="mt-0.5 text-sm text-ink/50">{serverQuery.data.name} · breeding & pal stats</p>
        </div>
        <div className="flex gap-1 rounded-xl bg-ink/5 p-1">
          <button className={segClass(mode === "breeding")} onClick={() => setMode("breeding")}>
            Breeding
          </button>
          <button className={segClass(mode === "stats")} onClick={() => setMode("stats")}>
            Stats
          </button>
        </div>
      </header>

      <div className="p-4 lg:p-8">
        {/* Mobile mode switch (the desktop one lives in the header). */}
        <div className="mb-4 flex gap-1 rounded-xl bg-ink/5 p-1 lg:hidden">
          <button className={cn(segClass(mode === "breeding"), "flex-1")} onClick={() => setMode("breeding")}>
            Breeding
          </button>
          <button className={cn(segClass(mode === "stats"), "flex-1")} onClick={() => setMode("stats")}>
            Stats
          </button>
        </div>

        {mode === "breeding" ? (
          <BreedingCalculator savePals={savePals} saveStatus={saveStatus} />
        ) : (
          <StatCalculator savePals={savePals} saveStatus={saveStatus} />
        )}
      </div>
    </div>
  );
}

function ElementChips({ characterId }: { characterId: string }) {
  const elements = (palEntry(characterId)?.elements ?? []).slice(0, 2);
  if (elements.length === 0) return null;
  return (
    <div className="mt-1 flex flex-wrap items-center justify-center gap-1">
      {elements.map((el) => (
        <span
          key={el}
          className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
          style={{ backgroundColor: `${elementColor(el)}22`, color: elementColor(el) }}
        >
          {el}
        </span>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Breeding
// ---------------------------------------------------------------------------

function BreedingCalculator({ savePals, saveStatus }: { savePals?: SavePal[]; saveStatus?: string }) {
  const [a, setA] = useState<PickedPal | null>(null);
  const [b, setB] = useState<PickedPal | null>(null);
  const [reverseTarget, setReverseTarget] = useState<string | null>(null);
  const [pickerFor, setPickerFor] = useState<PickTarget>(null);

  const child = a && b ? breedChild(a.characterId, b.characterId) : null;
  const bothFromSave = a?.save && b?.save;

  const onPick = (pick: PickedPal) => {
    if (pickerFor === "a") setA(pick);
    else if (pickerFor === "b") setB(pick);
    else if (pickerFor === "reverse") setReverseTarget(pick.characterId);
  };

  const pairs = reverseTarget ? parentPairsFor(reverseTarget) : [];

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-ink/10 bg-white/70 p-5 lg:p-8">
        <div className="grid grid-cols-[1fr_auto_1fr] items-start gap-3 lg:gap-6">
          <ParentSlot label="Parent" pick={a} onPick={() => setPickerFor("a")} onClear={() => setA(null)} />
          <div className="flex h-full items-center pt-8">
            <Egg />
          </div>
          <ParentSlot label="Parent" pick={b} onPick={() => setPickerFor("b")} onClear={() => setB(null)} />
        </div>

        <div className="my-6 flex items-center gap-3 text-ink/30">
          <div className="h-px flex-1 bg-ink/10" />
          <ArrowRight className="h-4 w-4 rotate-90" />
          <div className="h-px flex-1 bg-ink/10" />
        </div>

        {child ? (
          <div
            key={child.childId}
            className="mx-auto flex max-w-sm flex-col items-center motion-safe:animate-in motion-safe:zoom-in-95 motion-safe:duration-300"
          >
            {child.special && (
              <span className="mb-2 inline-flex items-center gap-1 rounded-full bg-brand-amber/15 px-2.5 py-0.5 text-xs font-semibold text-brand-amber">
                <Sparkles className="h-3 w-3" /> Special combo
              </span>
            )}
            <PalPortrait characterId={child.childId} size="lg" />
            <p className="mt-2 font-display text-xl font-extrabold">{palName(child.childId)}</p>
            <ElementChips characterId={child.childId} />
            {bothFromSave && <TalentTargets a={a!.save!} b={b!.save!} />}
          </div>
        ) : (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {a && b ? "These two can't be bred together." : "Pick two parents to see what they make."}
          </p>
        )}
      </section>

      {/* Reverse lookup */}
      <section className="rounded-2xl border border-ink/10 bg-white/70 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="font-display text-base font-bold">What breeds into…</h2>
            <p className="text-xs text-ink/40">Pick a target to see every parent pair that makes it.</p>
          </div>
          <button
            onClick={() => setPickerFor("reverse")}
            className="rounded-lg border border-ink/15 bg-white px-3 py-1.5 text-sm font-semibold text-ink transition hover:bg-ink/5"
          >
            {reverseTarget ? palName(reverseTarget) : "Choose a pal"}
          </button>
        </div>

        {reverseTarget && (
          <div className="mt-4">
            <p className="mb-2 text-xs text-ink/40">
              {pairs.length} {pairs.length === 1 ? "pair" : "pairs"} breed into{" "}
              <span className="font-semibold text-ink/70">{palName(reverseTarget)}</span>
            </p>
            <div className="grid max-h-96 grid-cols-1 gap-1.5 overflow-y-auto pr-1 sm:grid-cols-2">
              {pairs.map((p, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 rounded-xl border border-ink/10 bg-white/60 p-2"
                >
                  <PalPortrait characterId={p.aId} size="sm" />
                  <span className="text-ink/30">+</span>
                  <PalPortrait characterId={p.bId} size="sm" />
                  <div className="min-w-0 flex-1 text-xs">
                    <p className="truncate font-semibold text-foreground">{palName(p.aId)}</p>
                    <p className="truncate text-ink/50">{palName(p.bId)}</p>
                  </div>
                  {p.special && <Sparkles className="h-3.5 w-3.5 shrink-0 text-brand-amber" />}
                </div>
              ))}
              {pairs.length === 0 && (
                <p className="py-4 text-sm text-muted-foreground">
                  Nothing breeds into this one — it's only found in the wild.
                </p>
              )}
            </div>
          </div>
        )}
      </section>

      <PalPicker
        open={pickerFor !== null}
        onOpenChange={(o) => !o && setPickerFor(null)}
        onPick={onPick}
        title={pickerFor === "reverse" ? "Choose a target pal" : "Pick a parent"}
        savePals={savePals}
        saveStatus={saveStatus}
      />
    </div>
  );
}

function ParentSlot({
  label,
  pick,
  onPick,
  onClear,
}: {
  label: string;
  pick: PickedPal | null;
  onPick: () => void;
  onClear: () => void;
}) {
  if (!pick) {
    return (
      <button
        onClick={onPick}
        className="flex h-full min-h-[9rem] flex-col items-center justify-center gap-2 rounded-2xl border-2 border-dashed border-ink/15 p-4 text-ink/40 transition-colors hover:border-brand-red/40 hover:text-brand-red"
      >
        <span className="flex h-10 w-10 items-center justify-center rounded-full border-2 border-current text-xl">
          +
        </span>
        <span className="text-sm font-semibold">Pick a {label.toLowerCase()}</span>
      </button>
    );
  }
  const breedable = isBreedable(pick.characterId);
  return (
    <div className="relative flex flex-col items-center rounded-2xl border border-ink/10 bg-white/60 p-4">
      <button
        onClick={onClear}
        className="absolute right-2 top-2 rounded-full p-1 text-ink/30 hover:bg-ink/5 hover:text-ink"
        aria-label="Clear"
      >
        <X className="h-3.5 w-3.5" />
      </button>
      <button onClick={onPick} className="flex flex-col items-center" title="Change">
        <PalPortrait characterId={pick.characterId} size="lg" />
        <p className="mt-2 text-center font-display text-base font-bold leading-tight">
          {palName(pick.characterId)}
        </p>
      </button>
      {pick.save ? (
        <p className="mt-1 font-mono text-[11px] text-ink/40">
          Lv.{pick.save.level} · {pick.save.ivHp}/{pick.save.ivAttack}/{pick.save.ivDefense}
        </p>
      ) : (
        <ElementChips characterId={pick.characterId} />
      )}
      {!breedable && <p className="mt-1 text-[11px] font-semibold text-brand-red">Not breedable</p>}
    </div>
  );
}

/** Best talent a child could inherit from each parent — the target you breed
 * toward, since each talent is passed from one parent or rerolled. */
function TalentTargets({ a, b }: { a: SavePal; b: SavePal }) {
  const rows: [string, number, number][] = [
    ["HP", a.ivHp, b.ivHp],
    ["Attack", a.ivAttack, b.ivAttack],
    ["Defense", a.ivDefense, b.ivDefense],
  ];
  return (
    <div className="mt-4 w-full rounded-xl border border-ink/10 bg-ink/[0.03] p-3">
      <p className="mb-2 text-center text-[11px] font-semibold uppercase tracking-wide text-ink/40">
        Best inheritable talents
      </p>
      <div className="grid grid-cols-3 gap-2">
        {rows.map(([name, av, bv]) => (
          <div key={name} className="text-center">
            <p className="text-[11px] text-ink/40">{name}</p>
            <p className="font-mono text-lg font-bold text-foreground">{Math.max(av, bv)}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

/** The signature: a Palworld egg sitting between the two parents. */
function Egg() {
  return (
    <svg viewBox="0 0 40 52" className="h-14 w-10" aria-hidden="true">
      <defs>
        <linearGradient id="eggshell" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#FBF7EE" />
          <stop offset="1" stopColor="#EAD9B8" />
        </linearGradient>
      </defs>
      <path
        d="M20 2 C31 2 38 22 38 34 C38 44 30 50 20 50 C10 50 2 44 2 34 C2 22 9 2 20 2 Z"
        fill="url(#eggshell)"
        stroke="#D98C3F"
        strokeWidth="2"
      />
      {/* Spots, echoing the game's speckled eggs. */}
      <ellipse cx="14" cy="24" rx="3" ry="4" fill="#D98C3F" opacity="0.35" />
      <ellipse cx="25" cy="34" rx="4" ry="5" fill="#D98C3F" opacity="0.3" />
      <ellipse cx="17" cy="40" rx="2.5" ry="3" fill="#D98C3F" opacity="0.35" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

interface StatForm {
  characterId: string | null;
  level: number;
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
  soulHp: number;
  soulAttack: number;
  soulDefense: number;
  condenser: number;
  trust: number;
  passives: string[];
  isAlpha: boolean;
}

const emptyStatForm: StatForm = {
  characterId: null,
  level: 50,
  ivHp: 50,
  ivAttack: 50,
  ivDefense: 50,
  soulHp: 0,
  soulAttack: 0,
  soulDefense: 0,
  condenser: 0,
  trust: 0,
  passives: [],
  isAlpha: false,
};

function StatCalculator({ savePals, saveStatus }: { savePals?: SavePal[]; saveStatus?: string }) {
  const [form, setForm] = useState<StatForm>(emptyStatForm);
  const [pickerOpen, setPickerOpen] = useState(false);
  const set = <K extends keyof StatForm>(k: K, v: StatForm[K]) => setForm((f) => ({ ...f, [k]: v }));

  const onPick = (pick: PickedPal) => {
    if (pick.save) {
      const s = pick.save;
      setForm((f) => ({
        ...f,
        characterId: pick.characterId,
        level: s.level,
        ivHp: s.ivHp,
        ivAttack: s.ivAttack,
        ivDefense: s.ivDefense,
        condenser: s.condenser,
        soulHp: s.souls.hp,
        soulAttack: s.souls.attack,
        soulDefense: s.souls.defense,
        passives: s.passives,
        isAlpha: s.isAlpha,
        trust: s.trust,
      }));
    } else {
      // A bare species has no passives or alpha flag of its own to carry over.
      setForm((f) => ({ ...f, characterId: pick.characterId, passives: [], isAlpha: false }));
    }
  };

  const stats = form.characterId
    ? computeStats({
        characterId: form.characterId,
        level: form.level,
        ivHp: form.ivHp,
        ivAttack: form.ivAttack,
        ivDefense: form.ivDefense,
        soulHp: form.soulHp,
        soulAttack: form.soulAttack,
        soulDefense: form.soulDefense,
        condenser: form.condenser,
        trust: form.trust,
        passives: form.passives,
        isAlpha: form.isAlpha,
      })
    : null;
  const rating = talentRating(form.ivHp, form.ivAttack, form.ivDefense);
  const noStats = form.characterId && !hasCombatStats(form.characterId);
  // Passives that actually move the numbers, for the applied-effects list.
  const statPassives = form.passives.filter((c) => passiveStatEffect(c));

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <section className="space-y-5 rounded-2xl border border-ink/10 bg-white/70 p-5 lg:p-6">
        <div className="flex items-center gap-3">
          {form.characterId ? (
            <PalPortrait characterId={form.characterId} size="md" />
          ) : (
            <div className="flex h-14 w-14 items-center justify-center rounded-xl border-2 border-dashed border-ink/15 text-ink/30">
              ?
            </div>
          )}
          <div className="min-w-0 flex-1">
            <p className="font-display text-lg font-bold">
              {form.characterId ? palName(form.characterId) : "No pal chosen"}
            </p>
            <button
              onClick={() => setPickerOpen(true)}
              className="text-sm font-semibold text-brand-red hover:underline"
            >
              {form.characterId ? "Change pal" : "Pick a pal"}
            </button>
          </div>
        </div>

        <NumberField label="Level" min={1} max={60} value={form.level} onChange={(v) => set("level", v)} />

        <div>
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Talents (0–100)</p>
          <div className="grid grid-cols-3 gap-3">
            <NumberField label="HP" min={0} max={100} value={form.ivHp} onChange={(v) => set("ivHp", v)} />
            <NumberField label="Attack" min={0} max={100} value={form.ivAttack} onChange={(v) => set("ivAttack", v)} />
            <NumberField label="Defense" min={0} max={100} value={form.ivDefense} onChange={(v) => set("ivDefense", v)} />
          </div>
        </div>

        <div>
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">
            Upgrades <span className="font-normal normal-case text-ink/30">· optional</span>
          </p>
          <div className="grid grid-cols-3 gap-3">
            <NumberField label="Soul HP" min={0} max={10} value={form.soulHp} onChange={(v) => set("soulHp", v)} />
            <NumberField label="Soul Atk" min={0} max={10} value={form.soulAttack} onChange={(v) => set("soulAttack", v)} />
            <NumberField label="Soul Def" min={0} max={10} value={form.soulDefense} onChange={(v) => set("soulDefense", v)} />
          </div>
          <div className="mt-3 grid grid-cols-3 items-end gap-3">
            <NumberField label="Condenser ★" min={0} max={4} value={form.condenser} onChange={(v) => set("condenser", v)} />
            <NumberField label="Trust" min={0} max={10} value={form.trust} onChange={(v) => set("trust", v)} />
            <label className="flex cursor-pointer items-center gap-2 pb-2 text-sm text-foreground">
              <input
                type="checkbox"
                checked={form.isAlpha}
                onChange={(e) => set("isAlpha", e.target.checked)}
                className="h-4 w-4 accent-brand-red"
              />
              Alpha
            </label>
          </div>
        </div>

        {form.passives.length > 0 && (
          <div>
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Passives</p>
            <div className="flex flex-wrap gap-1.5">
              {form.passives.map((code) => {
                const eff = passiveStatEffect(code);
                const label = !eff
                  ? ""
                  : ["Atk", "Def", "HP"]
                      .map((n, i) => (eff[i] ? `${n} ${eff[i] > 0 ? "+" : ""}${eff[i]}%` : null))
                      .filter(Boolean)
                      .join(" · ");
                return (
                  <span
                    key={code}
                    className={cn(
                      "rounded-full px-2 py-0.5 text-[11px] font-medium",
                      eff ? "bg-brand-red/10 text-brand-red" : "bg-ink/5 text-ink/40",
                    )}
                    title={eff ? label : "No effect on the displayed stats"}
                  >
                    {passiveName(code)}
                    {eff && <span className="ml-1 font-mono">{label}</span>}
                  </span>
                );
              })}
            </div>
            {form.passives.length > statPassives.length && (
              <p className="mt-1.5 text-[11px] text-ink/35">
                Greyed passives boost element damage or buff you, not this pal's shown stats.
              </p>
            )}
          </div>
        )}
      </section>

      <section className="rounded-2xl border border-ink/10 bg-white/70 p-5 lg:p-6">
        <div className="flex items-center justify-between">
          <h2 className="font-display text-base font-bold">Estimated stats</h2>
          <span
            className={cn(
              "flex h-9 w-9 items-center justify-center rounded-lg font-display text-lg font-extrabold",
              rating.tier === "S"
                ? "bg-legendary/15 text-legendary"
                : rating.tier === "A"
                  ? "bg-pal-blue/15 text-pal-blue"
                  : rating.tier === "B"
                    ? "bg-pal-green/15 text-pal-green"
                    : "bg-ink/5 text-ink/50",
            )}
            title={`Talent rating · ${rating.average}% average`}
          >
            {rating.tier}
          </span>
        </div>

        {noStats ? (
          <p className="mt-6 text-sm text-muted-foreground">
            No base stats vendored for this pal — try another species.
          </p>
        ) : stats ? (
          <div className="mt-5 space-y-4">
            <StatBar label="HP" value={stats.hp} max={15000} color="#5B9E6F" />
            <StatBar label="Attack" value={stats.attack} max={1500} color="#E0502F" />
            <StatBar label="Defense" value={stats.defense} max={1500} color="#5B8DEF" />
            <p className="pt-2 text-[11px] text-ink/35">
              Calibrated against in-game values — Attack and Defense are exact. Trust is the one estimate, so a
              high-bond pal may read a touch low.
            </p>
          </div>
        ) : (
          <p className="mt-6 text-sm text-muted-foreground">Pick a pal to estimate its stats.</p>
        )}
      </section>

      <PalPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onPick={onPick}
        title="Pick a pal"
        savePals={savePals}
        saveStatus={saveStatus}
      />
    </div>
  );
}

function StatBar({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  const pct = Math.min(100, (value / max) * 100);
  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-sm font-semibold text-foreground">{label}</span>
        <span className="font-mono text-lg font-bold" style={{ color }}>
          {value.toLocaleString()}
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-ink/5">
        <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: color }} />
      </div>
    </div>
  );
}

function NumberField({
  label,
  value,
  onChange,
  min,
  max,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
}) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-ink/50">{label}</label>
      <NumberInput value={value} onChange={onChange} min={min} max={max} className="text-right" />
    </div>
  );
}
