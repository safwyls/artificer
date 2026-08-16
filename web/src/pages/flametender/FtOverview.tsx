import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { ServerPower } from "../../components/ServerPower";
import { FlameSigil } from "../../components/flametender/FlameSigil";
import { FtNote, FtPanel, FtStat, fkLogTone } from "../../components/flametender/FtPanel";
import { FtPlayerRows } from "./FtFlameborn";

// Enshrouded's hard slot cap — the fallback for a server whose Steam query
// hasn't answered. The configured slotCount is usually lower, and the query
// is the only source for it (the log never carries it), so drawing against
// the cap makes a full 4-slot server look quarter-empty.
const SLOT_CAP = 16;

function uptimeLabel(seconds: number | undefined): string {
  if (!seconds || seconds <= 0) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function agoLabel(ts: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(ts).getTime()) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function FtOverview() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { can, isAdmin } = useAuth();

  const serverQuery = useQuery({ queryKey: ["server", id], queryFn: () => api.getServer(id) });
  const infoQuery = useQuery({
    queryKey: ["server-info", id],
    queryFn: () => api.serverInfo(id),
    retry: false,
    refetchInterval: 15_000,
  });
  const playersQuery = useQuery({
    queryKey: ["server-players", id],
    queryFn: () => api.serverPlayers(id),
    refetchInterval: 10_000,
    retry: false,
  });
  const metricsQuery = useQuery({
    queryKey: ["server-metrics", id],
    queryFn: () => api.serverMetrics(id),
    refetchInterval: 30_000,
    retry: false,
  });
  const activityQuery = useQuery({
    queryKey: ["server-activity", id, 24],
    queryFn: () => api.serverActivity(id, 24),
    refetchInterval: 60_000,
  });
  const backupsQuery = useQuery({
    queryKey: ["backups", id],
    queryFn: () => api.listBackups(id),
    enabled: isAdmin,
  });
  const logsQuery = useQuery({
    queryKey: ["container-logs-preview", id],
    queryFn: () => api.containerLogs(id, 8),
    refetchInterval: 10_000,
    retry: false,
    enabled: can("power"),
  });

  if (serverQuery.isLoading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (serverQuery.isError || !serverQuery.data) return <p className="p-6 text-destructive">Server not found.</p>;

  const server = serverQuery.data;
  const info = infoQuery.data;
  const online = !infoQuery.isError && !!info;
  const players = playersQuery.data ?? [];
  // The game's own count when the Steam query answered, the log-derived roster
  // length otherwise. They differ when a player's join line has scrolled out of
  // the agent's ring — the roster can't name them, but they are on the server.
  const count = online ? (info?.playerCount ?? players.length) : 0;
  const slots = metricsQuery.data?.maxplayernum || SLOT_CAP;
  // A server is "running" for some time before it accepts joins; saying Online
  // then sends a friend to a connection error.
  const starting = online && info?.readiness === "starting";
  const uptime = metricsQuery.data?.uptime;
  const lastEvent = activityQuery.data?.events[0];
  const latestBackup = backupsQuery.data?.snapshots?.[0];
  // The community sizing observation, not a measurement of this host:
  // ~4.4 GB idle plus ~100 MB a player, growing with terrain edits
  // (docs/enshrouded-recon.md, "Runtime behavior").
  const memoryEstimate = Math.round((4.4 + count * 0.1) * 10) / 10;

  return (
    <div className="flametender min-h-full font-ftbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        {/* Hero */}
        <section
          aria-label="Server status"
          className="ft-toplight grid grid-cols-[96px,1fr] items-center gap-5 rounded-md border border-ft-edge bg-gradient-to-br from-ft-panel via-[#1a231d] to-[#151c17] px-5 py-5 sm:grid-cols-[132px,1fr] sm:px-6"
        >
          <FlameSigil lit={count} total={slots} online={online} size={132} />
          <div>
            <h2 className="font-ftdisplay text-2xl font-medium text-ft-bone sm:text-3xl">
              {server.name}
            </h2>
            <div className="ft-horizon mt-2" />
            <div className="mt-2 text-sm text-ft-lichen">
              {!online ? (
                "The flame is out — the server process is not running."
              ) : starting ? (
                "Kindling — the world is loading. The server won't accept joins until it finishes."
              ) : (
                <>
                  Uptime <b className="font-medium text-ft-stonehi">{uptimeLabel(uptime)}</b> · Port{" "}
                  <b className="font-medium text-ft-stonehi">{server.gamePort}/udp</b>
                </>
              )}
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              {/* Three states, not two. Flame means joinable and spore means
                  absent, so neither may stand for "up but not ready" — that one
                  is neutral stone with a pulse: no colour claim, and the motion
                  says it will pass. */}
              <span
                className={
                  starting
                    ? "rounded-sm border border-ft-edge px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-ft-stonehi"
                    : online
                      ? "rounded-sm border border-ft-flamedim px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-ft-flame"
                      : "rounded-sm border border-ft-sporedim px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-ft-spore"
                }
              >
                <span className={starting ? "inline-block animate-pulse" : undefined}>◈</span>{" "}
                {starting ? "Starting" : online ? "Online" : "Offline"}
              </span>
              {info?.version && (
                <span
                  className="rounded-sm border border-ft-edge px-2.5 py-0.5 font-mono text-[11.5px] text-ft-lichen"
                  title="The build the server is running — compare it with a friend's version-mismatch error"
                >
                  build {info.version}
                </span>
              )}
              <span className="rounded-sm border border-ft-edge px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-ft-lichen">
                Steam auth
              </span>
              <span className="rounded-sm border border-ft-edge px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-ft-lichen">
                {slots === SLOT_CAP ? `${SLOT_CAP}-slot cap` : `${slots} slots`}
              </span>
            </div>
          </div>
        </section>

        {/* Power — the agent is the only lever this game gives us, so the
            shared panel sits directly under the hero rather than in a
            settings corner. */}
        <ServerPower serverId={id} installPath={server.installPath} agentUrl={server.agentUrl} />

        {/* Vitals */}
        <section aria-label="Vitals" className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
          <FtStat
            label="Flameborn"
            value={online ? count : "—"}
            unit={`/ ${slots}`}
            hint={online ? `${Math.max(0, slots - count)} slots open` : "server offline"}
            meterPct={(count / slots) * 100}
          />
          <FtStat
            label="Memory guide"
            value={online ? memoryEstimate : "—"}
            unit="GB"
            hint="≈4 GB idle + ~100 MB × player; 16 GB floor"
            meterPct={(memoryEstimate / 16) * 100}
            warm
          />
          <FtStat label="Uptime" value={uptimeLabel(uptime)} hint={online ? "since last start" : "server offline"} />
          {isAdmin ? (
            <FtStat
              label="Latest backup"
              value={latestBackup ? agoLabel(latestBackup.ts) : "none"}
              hint={latestBackup ? latestBackup.name : "run one from World saves"}
            />
          ) : (
            <FtStat
              label="Last event"
              value={lastEvent ? agoLabel(lastEvent.ts) : "—"}
              hint={lastEvent ? `${lastEvent.name} ${lastEvent.event === "join" ? "joined" : "left"}` : "nothing in 24h"}
            />
          )}
        </section>

        <div className="grid grid-cols-1 items-start gap-3.5 lg:grid-cols-5">
          <div className="space-y-3.5 lg:col-span-3">
            <FtPanel
              title="Flameborn"
              meta={
                <Link to={`/servers/${id}/players`} className="text-ft-stonehi hover:text-ft-bone">
                  view all →
                </Link>
              }
              bodyClassName="pt-1.5"
            >
              <FtPlayerRows
                serverId={id}
                players={players}
                online={online}
                loading={playersQuery.isLoading}
                presentCount={info?.playerCount}
              />
            </FtPanel>

            {can("power") && (
              <FtPanel
                title="Server log"
                meta={
                  <Link to={`/servers/${id}/logs`} className="text-ft-stonehi hover:text-ft-bone">
                    open log →
                  </Link>
                }
              >
                <div className="max-h-[240px] overflow-y-auto rounded bg-ft-void px-3.5 py-2.5 font-mono text-xs leading-[1.75]">
                  {logsQuery.data?.lines?.length ? (
                    logsQuery.data.lines.map((line, i) => (
                      <div key={i} className={fkLogTone(line)}>
                        {line}
                      </div>
                    ))
                  ) : (
                    <span className="text-ft-lichen">
                      {logsQuery.isError ? "The log is out of reach — is the flameagent up?" : "No log lines yet."}
                    </span>
                  )}
                </div>
                <FtNote>
                  Enshrouded has no server console or admin API — everything
                  above is derived from the log tail, and moderation lives in
                  the in-game player menu.
                </FtNote>
              </FtPanel>
            )}
          </div>

          <div className="space-y-3.5 lg:col-span-2">
            <FtPanel title="World saves" meta="rolling copies + archives">
              {isAdmin ? (
                latestBackup ? (
                  <div className="space-y-1 text-sm">
                    <div className="font-mono text-xs text-ft-bone">{latestBackup.name}</div>
                    <div className="text-xs text-ft-lichen">{agoLabel(latestBackup.ts)}</div>
                    <Link to={`/servers/${id}/saves`} className="inline-block pt-1 text-xs text-ft-stonehi hover:text-ft-bone">
                      manage saves →
                    </Link>
                  </div>
                ) : (
                  <div className="text-sm text-ft-lichen">
                    No snapshots yet.{" "}
                    <Link to={`/servers/${id}/saves`} className="text-ft-stonehi hover:text-ft-bone">
                      Take the first →
                    </Link>
                  </div>
                )
              ) : (
                <div className="text-sm text-ft-lichen">Save snapshots are steward-only.</div>
              )}
            </FtPanel>

            <FtPanel title="Recent activity" meta="last 24h">
              {activityQuery.data?.events.length ? (
                <ul className="space-y-1.5 text-sm">
                  {activityQuery.data.events.slice(0, 6).map((e) => (
                    <li key={e.id} className="flex items-baseline justify-between gap-2">
                      <span>
                        <span
                          className={
                            e.event === "join"
                              ? "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-ft-ok"
                              : "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-[#3a4148]"
                          }
                        />
                        <b className="font-bold text-ft-bone">{e.name}</b>{" "}
                        <span className="text-ft-lichen">{e.event === "join" ? "joined" : "left"}</span>
                      </span>
                      <span className="whitespace-nowrap text-xs text-ft-lichen">{agoLabel(e.ts)}</span>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className="text-sm text-ft-lichen">Quiet above the fog — no joins or leaves in 24 hours.</div>
              )}
            </FtPanel>
          </div>
        </div>
      </div>
    </div>
  );
}
