import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Home, Users } from "lucide-react";
import { api, ApiError, type Guild, type Pal, type PlayerPals } from "../lib/api";
import { initials, playerColor } from "../lib/palette";
import { seenPhrase } from "../lib/time";
import { mapOf, MAP_AREAS } from "../lib/map";
import { nearestLandmark } from "../lib/pois";
import { DECK_BASE_ENTRIES, palDeckNo, palName } from "../lib/paldex";

function allPals(p: PlayerPals): Pal[] {
  return [...p.party, ...p.palbox, ...p.base, ...p.storage];
}

/** Base-species dex completion, matching the Paldex page's math. */
function dexBaseCount(labels: Set<string>): number {
  return DECK_BASE_ENTRIES.filter((e) => labels.has(e.label)).length;
}

function deckLabels(p: PlayerPals): Set<string> {
  const out = new Set<string>();
  for (const id of p.paldeck) {
    const label = palDeckNo(id);
    if (label) out.add(label);
  }
  return out;
}
import { ServerUnreachable } from "../components/ServerUnreachable";
import { SaveReadProgress } from "../components/SaveReadProgress";
import { SaveUpdatingBanner } from "../components/SaveUpdatingBanner";
import { SavePathSetup } from "../components/SavePathSetup";

/** "near <statue> · 320m", falling back to raw coordinates when nothing is
 * named nearby (which today means an empty landmark catalog, not distance —
 * every base has SOME nearest statue). Coordinates stay in the tooltip. */
function BaseWhereabouts({ x, y }: { x: number; y: number }) {
  const landmark = nearestLandmark(x, y);
  if (!landmark) {
    return (
      <p className="font-mono text-[11px] text-ink/40">
        {Math.round(x)}, {Math.round(y)}
      </p>
    );
  }
  return (
    <p
      className="truncate font-mono text-[11px] text-ink/40"
      title={`${Math.round(x)}, ${Math.round(y)}`}
    >
      near {landmark.name} · {landmark.meters}m
    </p>
  );
}

function GuildCard({ guild, players, serverId }: { guild: Guild; players: PlayerPals[]; serverId: number }) {
  const navigate = useNavigate();
  const byUid = new Map(players.map((p) => [p.uid, p]));
  // Uids match the player saves, so this normally hits directly. The name
  // fallback covers a member with no player save of their own.
  const byName = new Map(players.map((p) => [p.nickname.toLowerCase(), p]));
  const resolve = (uid: string, name: string) => byUid.get(uid) ?? byName.get(name.toLowerCase());

  const memberPlayers = guild.members
    .map((m) => resolve(m.uid, m.name))
    .filter((p): p is PlayerPals => p !== undefined);

  // Guild-wide rollup: what this guild owns and how much of the dex its
  // members have covered between them.
  const memberPals = memberPlayers.flatMap(allPals);
  const dexUnion = new Set<string>();
  for (const p of memberPlayers) for (const label of deckLabels(p)) dexUnion.add(label);
  const stats = {
    pals: memberPals.length,
    alphas: memberPals.filter((x) => x.isBoss && !x.isLucky).length,
    luckies: memberPals.filter((x) => x.isLucky).length,
    dexPct: Math.round((dexBaseCount(dexUnion) / DECK_BASE_ENTRIES.length) * 100),
  };

  // Sick pals stop working — the one base problem worth surfacing loudly.
  const sickWorkers = memberPals.filter((x) => x.baseId && x.sick);

  // Which pals work at which base, joined by camp id from the save.
  const workersByBase = new Map<string, Pal[]>();
  for (const p of players) {
    for (const pal of p.base) {
      if (!pal.baseId) continue;
      const list = workersByBase.get(pal.baseId);
      if (list) list.push(pal);
      else workersByBase.set(pal.baseId, [pal]);
    }
  }

  const statChip = "rounded-full border border-ink/10 bg-white px-2 py-0.5 font-mono text-[11px] text-ink/60";

  return (
    <section className="overflow-hidden rounded-2xl border border-ink/10 bg-white/70">
      <div className="flex flex-wrap items-center gap-3 border-b border-ink/10 px-5 py-4">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-brand-red/15 text-brand-red">
          <Users className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-display text-base font-bold">{guild.name || "Unnamed guild"}</h2>
          <p className="font-mono text-xs text-ink/40">
            Base level {guild.baseCampLevel} · {guild.memberCount}{" "}
            {guild.memberCount === 1 ? "member" : "members"} · {guild.bases.length}{" "}
            {guild.bases.length === 1 ? "base" : "bases"}
          </p>
        </div>
        <div className="flex w-full flex-wrap gap-1.5 sm:w-auto">
          <span className={statChip}>{stats.pals.toLocaleString()} pals</span>
          <span className={statChip}>{stats.alphas} alphas</span>
          <span className={statChip}>{stats.luckies} luckies</span>
          <span className={statChip}>{stats.dexPct}% dex</span>
          {sickWorkers.length > 0 && (
            <span
              className="rounded-full border border-brand-amber/50 bg-brand-amber/10 px-2 py-0.5 font-mono text-[11px] font-semibold text-ink/70"
              title={sickWorkers
                .slice(0, 12)
                .map((x) => `${x.nickname || palName(x.characterId)} — ${x.sick}`)
                .join(", ")}
            >
              {sickWorkers.length} sick worker{sickWorkers.length === 1 ? "" : "s"}
            </span>
          )}
        </div>
      </div>

      <div className="space-y-4 p-5">
        <div>
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">Members</p>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {guild.members.map((m) => {
              const player = resolve(m.uid, m.name);
              const seen = player ? seenPhrase(player) : "";
              const color = playerColor(player?.uid ?? m.uid);
              return (
                <div key={m.uid} className="flex items-center gap-2.5 rounded-xl border border-ink/10 bg-white/60 p-2.5">
                  <span
                    className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full font-display text-xs font-bold"
                    style={{ backgroundColor: `${color}33`, color }}
                  >
                    {initials(m.name || "?")}
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold text-foreground">{m.name || "Unknown"}</p>
                    <p className="font-mono text-[11px] text-ink/40">
                      {player ? `Lv.${player.level}` : "—"}
                      {seen && ` · ${seen}`}
                    </p>
                    {player && (
                      <p className="truncate font-mono text-[11px] text-ink/40">
                        {allPals(player).length.toLocaleString()} pals ·{" "}
                        {Math.round((dexBaseCount(deckLabels(player)) / DECK_BASE_ENTRIES.length) * 100)}% dex ·{" "}
                        {player.technologyPoints.toLocaleString()} tech
                      </p>
                    )}
                  </div>
                </div>
              );
            })}
            {guild.members.length === 0 && <p className="text-sm text-muted-foreground">No members recorded.</p>}
          </div>
        </div>

        {guild.bases.length > 0 && (
          <div>
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/40">
              Bases <span className="font-mono text-ink/30">({guild.bases.length})</span>
            </p>
            <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
              {guild.bases.map((b, i) => {
                const workers = workersByBase.get(b.id) ?? [];
                const sick = workers.filter((w) => w.sick);
                return (
                <div
                  key={i}
                  className="flex items-center gap-3 rounded-xl border border-ink/10 bg-white/60 p-2.5"
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-brand-amber/15">
                    <Home className="h-4 w-4 text-brand-amber" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold text-foreground">
                      Base {i + 1}
                      <span className="ml-1.5 text-xs font-normal text-ink/45">
                        {MAP_AREAS[mapOf(b.x, b.y)].label}
                      </span>
                    </p>
                    <BaseWhereabouts x={b.x} y={b.y} />
                    <p
                      className="truncate font-mono text-[11px] text-ink/40"
                      title={workers
                        .slice(0, 15)
                        .map((w) => (w.nickname ? `${w.nickname} (${palName(w.characterId)})` : palName(w.characterId)))
                        .join(", ")}
                    >
                      {workers.length === 0 ? "no workers" : `${workers.length} workers`}
                      {sick.length > 0 && (
                        <span className="font-semibold text-destructive"> · {sick.length} sick</span>
                      )}
                    </p>
                  </div>
                  <button
                    onClick={() =>
                      navigate(
                        `/servers/${serverId}/map?base=${encodeURIComponent(`base-${guild.id}-${i}`)}` +
                          `&bx=${b.x}&by=${b.y}`,
                      )
                    }
                    className="shrink-0 text-xs font-semibold text-pal-blue hover:underline"
                  >
                    View on map
                  </button>
                </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}

export function ServerGuilds() {
  const { serverID } = useParams();
  const id = Number(serverID);

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  const infoQuery = useQuery({ queryKey: ["server-info", id], queryFn: () => api.serverInfo(id), retry: false });
  const guildsQuery = useQuery({
    queryKey: ["server-guilds", id],
    queryFn: () => api.serverGuilds(id),
    retry: false,
    refetchInterval: 5 * 60_000,
    // Keep the parse across navigation; re-reading a large save is slow (see
    // ServerPlayers for the same reasoning). This key is shared with the live
    // map's marker overlay, so both share one cached read.
    gcTime: 60 * 60_000,
    staleTime: 60_000,
  });
  // Deliberately NOT showing a "bases X/max" cap: BaseCampMaxNumInGuild
  // from live settings is not the effective limit on current game builds —
  // verified on a real server that allowed 8 bases while the ini said 4.
  // Since 1.0 the slots unlock via Base Missions raising the base level,
  // whose table isn't in any data we vendor.

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const notConfigured =
    guildsQuery.isError && guildsQuery.error instanceof ApiError && guildsQuery.error.status === 400;
  const hasData = guildsQuery.data !== undefined;
  const guilds = guildsQuery.data?.guilds ?? [];
  const players = guildsQuery.data?.players ?? [];

  return (
    <div>
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Guilds</h1>
          <p className="mt-0.5 text-sm text-ink/50">{serverQuery.data.name} · from the save file</p>
        </div>
      </header>

      <div className="space-y-4 p-4 lg:space-y-6 lg:p-8">
        {!hasData && guildsQuery.isFetching && <SaveReadProgress />}
        {notConfigured && !hasData && <SavePathSetup />}

        {!hasData && guildsQuery.isError && !notConfigured && (
          infoQuery.isError ? <ServerUnreachable /> : (
            <p className="text-sm text-destructive">
              Could not read the save file: {(guildsQuery.error as Error).message}
            </p>
          )
        )}

        {hasData && guildsQuery.isFetching && <SaveUpdatingBanner />}

        {hasData &&
          (guilds.length === 0 ? (
            <p className="text-sm text-muted-foreground">No guilds in this save yet.</p>
          ) : (
            guilds.map((g) => <GuildCard key={g.id} guild={g} players={players} serverId={id} />)
          ))}
      </div>
    </div>
  );
}
