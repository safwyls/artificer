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

function CompletionRow({ player }: { player: PlayerPals }) {
  const [open, setOpen] = useState(false);
  const caught = useMemo(() => deckLabels(player), [player]);
  const total = DECK_ENTRIES.length;
  const pct = total ? Math.round((caught.size / total) * 100) : 0;
  const missing = useMemo(() => DECK_ENTRIES.filter((e) => !caught.has(e.label)), [caught]);

  return (
    <li>
      <button
        className="flex w-full items-center gap-4 px-5 py-3.5 text-left hover:bg-ink/[0.02]"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">{player.nickname || player.uid.slice(0, 8)}</span>
          <span className="font-mono text-xs text-ink/40">Lv.{player.level}</span>
        </span>
        <span className="hidden h-2 w-40 overflow-hidden rounded-full bg-ink/10 sm:block lg:w-64">
          <span className="block h-full rounded-full bg-brand-red" style={{ width: `${pct}%` }} />
        </span>
        <span className="w-24 text-right font-mono text-sm tabular-nums">
          {caught.size}/{total}
        </span>
        <span className="w-12 text-right font-mono text-sm font-semibold tabular-nums">{pct}%</span>
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-ink/30 transition-transform", open && "rotate-180")} />
      </button>

      {open && (
        <div className="border-t border-ink/5 bg-ink/[0.015] px-5 py-4">
          {missing.length === 0 ? (
            <p className="text-sm text-ink/60">Paldex complete — every species is registered. 🎉</p>
          ) : (
            <>
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/35">
                Missing · {missing.length}
              </p>
              <div className="flex max-h-64 flex-wrap gap-1.5 overflow-y-auto">
                {missing.map((e) => (
                  <span
                    key={e.label}
                    className="flex items-center gap-1.5 rounded-lg border border-ink/10 bg-white py-1 pl-1 pr-2"
                    title={palName(e.characterId)}
                  >
                    <img src={palIconUrl(e.characterId)} alt="" loading="lazy" className="h-5 w-5 object-contain" />
                    <span className="font-mono text-[10px] text-ink/40">#{e.label}</span>
                    <span className="max-w-[7rem] truncate text-xs text-ink/70">{palName(e.characterId)}</span>
                  </span>
                ))}
              </div>
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

  const total = DECK_ENTRIES.length;

  const serverCaught = useMemo(() => {
    const union = new Set<string>();
    for (const p of players) for (const label of deckLabels(p)) union.add(label);
    return union;
  }, [players]);

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
      .slice(0, 5);

    const captures = players
      .map((p) => ({
        name: p.nickname || p.uid.slice(0, 8),
        total: Object.values(p.captures).reduce((n, c) => n + c, 0),
        species: Object.keys(p.captures).length,
      }))
      .filter((c) => c.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 5);

    const hunters = players
      .map((p) => {
        const pals = allPals(p);
        return {
          name: p.nickname || p.uid.slice(0, 8),
          alphas: pals.filter((x) => x.isBoss).length,
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
      .slice(0, 8);

    return { best, captures, hunters, rarest };
  }, [players]);

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const notConfigured = palsQuery.isError && palsQuery.error instanceof ApiError && palsQuery.error.status === 400;
  const pct = total ? Math.round((serverCaught.size / total) * 100) : 0;

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
            {/* The one bold element: how much of the Paldex this server has
                seen, all players together. */}
            <section className="clip-notch-lg border border-ink/10 bg-white px-6 py-5 lg:px-8">
              <p className="text-xs font-bold uppercase tracking-widest text-ink/50">Server Paldex</p>
              <div className="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span className="font-display text-4xl font-extrabold lg:text-5xl">{pct}%</span>
                <span className="font-mono text-sm text-ink/50">
                  {serverCaught.size} of {total} species registered by someone
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
            <div className="grid gap-4 lg:grid-cols-2">
              <RecordCard icon={<Sparkles className="h-4 w-4 text-brand-amber" />} title="Best pals · IV total">
                {records.best.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No pals in the save yet.</p>
                ) : (
                  <ul className="divide-y divide-ink/5">
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

              <RecordCard icon={<Crown className="h-4 w-4 text-legendary" />} title="One of a kind">
                {records.rarest.length === 0 ? (
                  <p className="py-2 text-sm text-ink/60">No species is down to a single specimen.</p>
                ) : (
                  <>
                    <p className="pb-1 text-xs text-ink/45">Species with exactly one specimen on the server.</p>
                    <ul className="divide-y divide-ink/5">
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
            </div>
          </>
        )}
      </div>
    </div>
  );
}
