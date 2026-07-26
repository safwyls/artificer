import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Crown, Sparkles, Swords, Target } from "lucide-react";
import { api, ApiError, type Pal, type PlayerPals } from "../lib/api";
import { DECK_ENTRIES, palDeckNo, palIconUrl, palName } from "../lib/paldex";
import { initials, playerColor } from "../lib/palette";
import { cn } from "../lib/utils";
import { PalPortrait } from "../components/PalPortrait";
import { TalentTriplet } from "../components/TalentTriplet";
import { SavePathSetup } from "../components/SavePathSetup";
import { ServerUnreachable } from "../components/ServerUnreachable";

/** All of a player's pals, wherever they live. */
function allPals(p: PlayerPals): Pal[] {
  return [...p.party, ...p.palbox, ...p.base, ...p.storage];
}

/** A player's registered deck labels. The save's record ids normalize
 * through palDeckNo, so decorated capture ids count for their species. */
function deckLabels(p: PlayerPals): Set<string> {
  const out = new Set<string>();
  for (const id of p.paldeck) {
    const label = palDeckNo(id);
    if (label) out.add(label);
  }
  return out;
}

/** Deck labels of the species a player currently owns, for flagging
 * "in the box but never registered" (a traded-in pal doesn't write the
 * receiver's dex — verified against a real save). */
function ownedLabels(p: PlayerPals): Set<string> {
  const out = new Set<string>();
  for (const pal of allPals(p)) {
    const label = palDeckNo(pal.characterId);
    if (label) out.add(label);
  }
  return out;
}

// Completion percentages track the numbered entries, like the game's own
// counter — B-subspecies sit under the same number in-game and are shown
// separately rather than dragging the headline down.
const BASE_ENTRIES = DECK_ENTRIES.filter((e) => /^\d+$/.test(e.label));
const VARIANT_ENTRIES = DECK_ENTRIES.filter((e) => !/^\d+$/.test(e.label));

function PlayerChip({ name }: { name: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full text-[8px] font-bold text-paper"
        style={{ backgroundColor: playerColor(name) }}
      >
        {initials(name)}
      </span>
      <span className="truncate">{name}</span>
    </span>
  );
}

function MissingChip({ entry, owned }: { entry: { label: string; characterId: string }; owned: boolean }) {
  return (
    <span
      className={cn(
        "flex items-center gap-1.5 rounded-lg border bg-white py-1 pl-1 pr-2",
        owned ? "border-brand-amber/60 ring-1 ring-brand-amber/40" : "border-ink/10",
      )}
      title={
        owned
          ? `${palName(entry.characterId)} — in their box but not registered; the dex only counts pals they acquired themselves (a traded pal doesn't register).`
          : palName(entry.characterId)
      }
    >
      <img src={palIconUrl(entry.characterId)} alt="" loading="lazy" className="h-5 w-5 object-contain" />
      <span className="font-mono text-[10px] text-ink/40">#{entry.label}</span>
      <span className="max-w-[7rem] truncate text-xs text-ink/70">{palName(entry.characterId)}</span>
      {owned && <span className="text-[10px] font-semibold text-brand-amber">owned</span>}
    </span>
  );
}

function CompletionRow({ player }: { player: PlayerPals }) {
  const [open, setOpen] = useState(false);
  const caught = useMemo(() => deckLabels(player), [player]);
  const owned = useMemo(() => ownedLabels(player), [player]);
  const caughtBase = useMemo(() => BASE_ENTRIES.filter((e) => caught.has(e.label)).length, [caught]);
  const total = BASE_ENTRIES.length;
  const pct = total ? Math.round((caughtBase / total) * 100) : 0;
  const missingBase = useMemo(() => BASE_ENTRIES.filter((e) => !caught.has(e.label)), [caught]);
  const missingVariants = useMemo(() => VARIANT_ENTRIES.filter((e) => !caught.has(e.label)), [caught]);
  const ownedUnregistered = useMemo(
    () => [...missingBase, ...missingVariants].filter((e) => owned.has(e.label)).length,
    [missingBase, missingVariants, owned],
  );
  // A player file with no dex record reads as zero registered while they
  // plainly own pals — that's missing data, not a 0% player.
  const noRecord = player.paldeck.length === 0 && allPals(player).length > 0;

  return (
    <li>
      <button
        className="flex w-full items-center gap-4 px-5 py-3.5 text-left hover:bg-ink/[0.02]"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        disabled={noRecord}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">{player.nickname || player.uid.slice(0, 8)}</span>
          <span className="font-mono text-xs text-ink/40">Lv.{player.level}</span>
        </span>
        {noRecord ? (
          <span className="text-xs text-ink/45">no Paldex record in the save</span>
        ) : (
          <>
            <span className="hidden h-2 w-40 overflow-hidden rounded-full bg-ink/10 sm:block lg:w-64">
              <span className="block h-full rounded-full bg-brand-red" style={{ width: `${pct}%` }} />
            </span>
            <span className="w-24 text-right font-mono text-sm tabular-nums">
              {caughtBase}/{total}
            </span>
            <span className="w-12 text-right font-mono text-sm font-semibold tabular-nums">{pct}%</span>
            <ChevronDown className={cn("h-4 w-4 shrink-0 text-ink/30 transition-transform", open && "rotate-180")} />
          </>
        )}
      </button>

      {open && !noRecord && (
        <div className="space-y-3 border-t border-ink/5 bg-ink/[0.015] px-5 py-4">
          {missingBase.length === 0 && missingVariants.length === 0 ? (
            <p className="text-sm text-ink/60">Paldex complete — every species and variant is registered. 🎉</p>
          ) : (
            <>
              {ownedUnregistered > 0 && (
                <p className="text-xs text-ink/50">
                  <span className="font-semibold text-brand-amber">{ownedUnregistered}</span> of these are in their
                  box but unregistered — the dex only counts pals a player acquired themselves, so traded-in pals
                  don't register.
                </p>
              )}
              {missingBase.length > 0 && (
                <div>
                  <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/35">
                    Missing · {missingBase.length}
                  </p>
                  <div className="flex max-h-64 flex-wrap gap-1.5 overflow-y-auto">
                    {missingBase.map((e) => (
                      <MissingChip key={e.label} entry={e} owned={owned.has(e.label)} />
                    ))}
                  </div>
                </div>
              )}
              {missingVariants.length > 0 && (
                <div>
                  <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/35">
                    Variants missing · {missingVariants.length}
                    <span className="ml-1 normal-case text-ink/30">(not counted in the %)</span>
                  </p>
                  <div className="flex max-h-40 flex-wrap gap-1.5 overflow-y-auto">
                    {missingVariants.map((e) => (
                      <MissingChip key={e.label} entry={e} owned={owned.has(e.label)} />
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </li>
  );
}

function RecordCard({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-xl border border-ink/10 bg-white">
      <div className="flex items-center gap-2 border-b border-ink/5 px-5 py-3.5">
        {icon}
        <h3 className="font-display text-sm font-bold">{title}</h3>
      </div>
      <div className="px-5 py-3">{children}</div>
    </section>
  );
}

export function ServerPaldex() {
  const { serverID } = useParams();
  const id = Number(serverID);

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  const infoQuery = useQuery({ queryKey: ["server-info", id], queryFn: () => api.serverInfo(id), retry: false });
  // Shares the pal viewer's cache — opening Paldex after Player pals is free.
  const palsQuery = useQuery({
    queryKey: ["server-pals", id],
    queryFn: () => api.serverPals(id),
    retry: false,
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });

  const players = useMemo(
    () => (palsQuery.data?.players ?? []).filter((p) => p.nickname || allPals(p).length > 0),
    [palsQuery.data],
  );

  const serverCaught = useMemo(() => {
    const union = new Set<string>();
    for (const p of players) for (const label of deckLabels(p)) union.add(label);
    return union;
  }, [players]);
  const serverCaughtBase = useMemo(
    () => BASE_ENTRIES.filter((e) => serverCaught.has(e.label)).length,
    [serverCaught],
  );
  const serverCaughtVariants = useMemo(
    () => VARIANT_ENTRIES.filter((e) => serverCaught.has(e.label)).length,
    [serverCaught],
  );

  // Every player showing an empty dex while owning pals means the save's
  // Players/*.sav records weren't readable — say so instead of rendering a
  // page of zeros that looks like nobody ever caught anything.
  const recordsUnavailable =
    players.length > 0 &&
    players.every((p) => p.paldeck.length === 0) &&
    players.some((p) => allPals(p).length > 0);

  const records = useMemo(() => {
    const owned: { pal: Pal; owner: string }[] = [];
    for (const p of players) {
      const owner = p.nickname || p.uid.slice(0, 8);
      for (const pal of allPals(p)) owned.push({ pal, owner });
    }

    const best = [...owned]
      .sort(
        (a, b) =>
          b.pal.talentHp + b.pal.talentShot + b.pal.talentDefense - (a.pal.talentHp + a.pal.talentShot + a.pal.talentDefense),
      )
      .slice(0, 25);

    const captures = players
      .map((p) => {
        // PalCaptureCount keys are raw ids — BOSS_ variants and captured
        // HUMANS included. Fold everything through the deck so a species
        // counts once, and humans stay off a pal leaderboard.
        const byLabel = new Map<string, number>();
        for (const [cid, n] of Object.entries(p.captures)) {
          const label = palDeckNo(cid);
          if (label) byLabel.set(label, (byLabel.get(label) ?? 0) + n);
        }
        let total = 0;
        for (const n of byLabel.values()) total += n;
        return { name: p.nickname || p.uid.slice(0, 8), total, species: byLabel.size };
      })
      .filter((c) => c.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 5);

    const hunters = players
      .map((p) => {
        const pals = allPals(p);
        return {
          name: p.nickname || p.uid.slice(0, 8),
          // The game stores luckies with the BOSS_ prefix too, so "boss"
          // alone would count every lucky as an alpha as well.
          alphas: pals.filter((x) => x.isBoss && !x.isLucky).length,
          luckies: pals.filter((x) => x.isLucky).length,
        };
      })
      .filter((h) => h.alphas + h.luckies > 0)
      .sort((a, b) => b.alphas + b.luckies - (a.alphas + a.luckies))
      .slice(0, 5);

    // Species exactly one specimen of exists on the whole server — the
    // catches nobody else has.
    const countByLabel = new Map<string, { n: number; owner: string; characterId: string }>();
    for (const { pal, owner } of owned) {
      const label = palDeckNo(pal.characterId);
      if (!label) continue;
      const cur = countByLabel.get(label);
      if (cur) cur.n += 1;
      else countByLabel.set(label, { n: 1, owner, characterId: pal.characterId });
    }
    const rarest = [...countByLabel.entries()]
      .filter(([, v]) => v.n === 1)
      .map(([label, v]) => ({ label, ...v }))
      .sort((a, b) => parseInt(b.label, 10) - parseInt(a.label, 10))
      .slice(0, 25);

    return { best, captures, hunters, rarest };
  }, [players]);

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const notConfigured = palsQuery.isError && palsQuery.error instanceof ApiError && palsQuery.error.status === 400;
  const baseTotal = BASE_ENTRIES.length;
  const pct = baseTotal ? Math.round((serverCaughtBase / baseTotal) * 100) : 0;

  return (
    <div className="pb-24">
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Paldex</h1>
          <p className="text-sm text-ink/60">Completion per player, and the server's record book</p>
        </div>
      </header>

      <div className="mx-auto max-w-5xl space-y-4 p-4 lg:space-y-6 lg:p-8">
        {notConfigured && <SavePathSetup />}
        {palsQuery.isError && !notConfigured && (infoQuery.isError ? <ServerUnreachable /> : (
          <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-ink/70">
            The save could not be read. Refresh to try again.
          </p>
        ))}
        {palsQuery.isLoading && <p className="p-2 text-muted-foreground">Reading the save…</p>}

        {palsQuery.data && (
          <>
            {recordsUnavailable && (
              <p className="rounded-lg border border-brand-amber/50 bg-brand-amber/10 px-4 py-3 text-sm text-ink/70">
                No Paldex records were found in the save — completion can't be computed. The records live in the
                world folder's <code className="font-mono">Players/*.sav</code> files, so make sure the server's
                save path mounts the whole folder, not just Level.sav.
              </p>
            )}

            {/* The one bold element: how much of the Paldex this server has
                seen, all players together. */}
            <section className="clip-notch-lg rounded-br-[10px] rounded-tl-[10px] border border-ink/10 bg-white px-6 py-5 lg:px-8">
              <p className="text-xs font-bold uppercase tracking-widest text-ink/50">Server Paldex</p>
              <div className="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span className="font-display text-4xl font-extrabold lg:text-5xl">{pct}%</span>
                <span className="font-mono text-sm text-ink/50">
                  {serverCaughtBase} of {baseTotal} species registered by someone
                  {serverCaughtVariants > 0 && ` · +${serverCaughtVariants}/${VARIANT_ENTRIES.length} variants`}
                </span>
              </div>
              <div className="mt-3 h-2.5 w-full overflow-hidden rounded-full bg-ink/10">
                <div className="h-full rounded-full bg-brand-red" style={{ width: `${pct}%` }} />
              </div>
            </section>

            <section className="rounded-xl border border-ink/10 bg-white">
              <div className="border-b border-ink/5 px-5 py-4">
                <h2 className="font-display text-base font-bold">Completion by player</h2>
              </div>
              {players.length === 0 ? (
                <p className="px-5 py-6 text-sm text-ink/60">No players in the save yet.</p>
              ) : (
                <ul className="divide-y divide-ink/5">
                  {players.map((p) => (
                    <CompletionRow key={p.uid} player={p} />
                  ))}
                </ul>
              )}
            </section>

            <h2 className="pt-2 font-display text-base font-bold">Server records</h2>
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              <RecordCard icon={<Sparkles className="h-4 w-4 text-brand-amber" />} title="Best pals · IV total">
                {records.best.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No pals in the save yet.</p>
                ) : (
                  // Top 25, scrolling after roughly the first six — the
                  // half-visible row is the scroll affordance.
                  <ul className="max-h-80 divide-y divide-ink/5 overflow-y-auto pr-1">
                    {records.best.map(({ pal, owner }) => (
                      <li key={pal.instanceId} className="flex items-center gap-3 py-2">
                        <PalPortrait characterId={pal.characterId} size="sm" />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-semibold">
                            {pal.nickname || palName(pal.characterId)}
                          </span>
                          <span className="block truncate text-xs text-ink/45">
                            <PlayerChip name={owner} />
                          </span>
                        </span>
                        <TalentTriplet hp={pal.talentHp} attack={pal.talentShot} defense={pal.talentDefense} />
                      </li>
                    ))}
                  </ul>
                )}
              </RecordCard>

              <RecordCard icon={<Crown className="h-4 w-4 text-legendary" />} title="One of a kind">
                {records.rarest.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No species is down to a single specimen.</p>
                ) : (
                  <>
                    <p className="pb-1 text-xs text-ink/45">Species with exactly one specimen on the server.</p>
                    <ul className="max-h-80 divide-y divide-ink/5 overflow-y-auto pr-1">
                      {records.rarest.map((r) => (
                        <li key={r.label} className="flex items-center gap-3 py-2 text-sm">
                          <PalPortrait characterId={r.characterId} size="sm" />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate font-semibold">
                              <span className="font-mono text-xs text-ink/40">#{r.label}</span>{" "}
                              {palName(r.characterId)}
                            </span>
                            <span className="block truncate text-xs text-ink/45">
                              <PlayerChip name={r.owner} />
                            </span>
                          </span>
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </RecordCard>

              <RecordCard icon={<Swords className="h-4 w-4 text-brand-red" />} title="Alphas & luckies owned">
                {records.hunters.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No alpha or lucky pals owned yet.</p>
                ) : (
                  <ul className="divide-y divide-ink/5">
                    {records.hunters.map((h) => (
                      <li key={h.name} className="flex items-center gap-3 py-2 text-sm">
                        <span className="min-w-0 flex-1 truncate font-semibold">
                          <PlayerChip name={h.name} />
                        </span>
                        <span className="font-mono text-xs tabular-nums text-brand-red">{h.alphas} alpha</span>
                        <span className="w-20 text-right font-mono text-xs tabular-nums text-brand-amber">
                          {h.luckies} lucky
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </RecordCard>

              <RecordCard icon={<Target className="h-4 w-4 text-pal-blue" />} title="Most captures">
                {records.captures.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No captures recorded yet.</p>
                ) : (
                  <ul className="divide-y divide-ink/5">
                    {records.captures.map((c, i) => (
                      <li key={c.name} className="flex items-center gap-3 py-2 text-sm">
                        <span className="w-5 font-mono text-xs text-ink/35">{i + 1}.</span>
                        <span className="min-w-0 flex-1 truncate font-semibold">
                          <PlayerChip name={c.name} />
                        </span>
                        <span className="font-mono text-xs text-ink/45">{c.species} species</span>
                        <span className="w-16 text-right font-mono font-semibold tabular-nums">{c.total}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </RecordCard>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
