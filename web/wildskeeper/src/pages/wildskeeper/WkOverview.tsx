import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { ServerPower } from "../../components/ServerPower";
import { RuneSigil } from "../../components/wildskeeper/RuneSigil";
import { WkNote, WkPanel, WkStat, wkLogTone } from "../../components/wildskeeper/WkPanel";
import { WkPlayerRows } from "./WkAdventurers";

const MAX_PLAYERS = 6;

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

export function WkOverview() {
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
  const online = !infoQuery.isError && !!infoQuery.data;
  const players = playersQuery.data ?? [];
  const count = online ? players.length : 0;
  const uptime = metricsQuery.data?.uptime;
  const lastEvent = activityQuery.data?.events[0];
  const latestBackup = backupsQuery.data?.snapshots?.[0];
  // The official sizing rule, not a measurement: 2 GB base + 1 GB a player.
  const memoryEstimate = 2 + count;

  return (
    <div className="wildskeeper min-h-full font-wkbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        {/* Hero */}
        <section
          aria-label="Server status"
          className="wk-corners grid grid-cols-[96px,1fr] items-center gap-5 rounded-md border border-wk-brass bg-gradient-to-br from-wk-panel via-[#161d28] to-[#131a24] px-5 py-5 sm:grid-cols-[132px,1fr] sm:px-6"
        >
          <RuneSigil lit={count} total={MAX_PLAYERS} online={online} size={132} />
          <div>
            <h2 className="font-wkdisplay text-xl font-semibold tracking-[0.04em] text-wk-parchment sm:text-2xl">
              {server.name}
            </h2>
            <div className="mt-1 text-sm text-wk-mist">
              {online ? (
                <>
                  Uptime <b className="font-medium text-wk-brasshi">{uptimeLabel(uptime)}</b> · Port{" "}
                  <b className="font-medium text-wk-brasshi">{server.gamePort}/udp</b>
                </>
              ) : (
                "The keep is dark — the server process is not running."
              )}
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              <span
                className={
                  online
                    ? "rounded-sm border border-wk-runedim px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-wk-rune"
                    : "rounded-sm border border-wk-emberdim px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-wk-ember"
                }
              >
                ◈ {online ? "Online" : "Offline"}
              </span>
              {infoQuery.data?.version && (
                <span className="rounded-sm border border-wk-edge px-2.5 py-0.5 font-mono text-[11.5px] text-wk-mist">
                  {infoQuery.data.version}
                </span>
              )}
              <span className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-wk-mist">
                Epic auth
              </span>
              <span className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-[11.5px] uppercase tracking-[0.08em] text-wk-mist">
                {MAX_PLAYERS}-player cap
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
          <WkStat
            label="Adventurers"
            value={online ? count : "—"}
            unit={`/ ${MAX_PLAYERS}`}
            hint={online ? `${MAX_PLAYERS - count} slots open` : "server offline"}
            meterPct={(count / MAX_PLAYERS) * 100}
          />
          <WkStat
            label="Memory guide"
            value={online ? memoryEstimate : "—"}
            unit="GB"
            hint="2 GB base + 1 GB × adventurer"
            meterPct={(memoryEstimate / (2 + MAX_PLAYERS)) * 100}
            warm
          />
          <WkStat label="Uptime" value={uptimeLabel(uptime)} hint={online ? "since last start" : "server offline"} />
          {isAdmin ? (
            <WkStat
              label="Latest backup"
              value={latestBackup ? agoLabel(latestBackup.ts) : "none"}
              hint={latestBackup ? latestBackup.name : "run one from World saves"}
            />
          ) : (
            <WkStat
              label="Last event"
              value={lastEvent ? agoLabel(lastEvent.ts) : "—"}
              hint={lastEvent ? `${lastEvent.name} ${lastEvent.event === "join" ? "joined" : "left"}` : "nothing in 24h"}
            />
          )}
        </section>

        <div className="grid grid-cols-1 items-start gap-3.5 lg:grid-cols-5">
          <div className="space-y-3.5 lg:col-span-3">
            <WkPanel
              title="Adventurers"
              meta={
                <Link to={`/servers/${id}/players`} className="text-wk-brasshi hover:text-wk-parchment">
                  view all →
                </Link>
              }
              bodyClassName="pt-1.5"
            >
              <WkPlayerRows serverId={id} players={players} online={online} loading={playersQuery.isLoading} />
            </WkPanel>

            {can("power") && (
              <WkPanel
                title="Server log"
                meta={
                  <Link to={`/servers/${id}/logs`} className="text-wk-brasshi hover:text-wk-parchment">
                    open log →
                  </Link>
                }
              >
                <div className="max-h-[240px] overflow-y-auto rounded bg-wk-ink px-3.5 py-2.5 font-mono text-xs leading-[1.75]">
                  {logsQuery.data?.lines?.length ? (
                    logsQuery.data.lines.map((line, i) => (
                      <div key={i} className={wkLogTone(line)}>
                        {line}
                      </div>
                    ))
                  ) : (
                    <span className="text-wk-mist">
                      {logsQuery.isError ? "The log is out of reach — is the wkagent up?" : "No log lines yet."}
                    </span>
                  )}
                </div>
                <WkNote>
                  No native console — command execution needs the <code className="font-mono not-italic text-wk-rune">dwbridge</code>{" "}
                  mod. Everything above is log-tail and config driven.
                </WkNote>
              </WkPanel>
            )}
          </div>

          <div className="space-y-3.5 lg:col-span-2">
            <WkPanel title="World saves" meta="ZFS-friendly archives">
              {isAdmin ? (
                latestBackup ? (
                  <div className="space-y-1 text-sm">
                    <div className="font-mono text-xs text-wk-parchment">{latestBackup.name}</div>
                    <div className="text-xs text-wk-mist">{agoLabel(latestBackup.ts)}</div>
                    <Link to={`/servers/${id}/saves`} className="inline-block pt-1 text-xs text-wk-brasshi hover:text-wk-parchment">
                      manage saves →
                    </Link>
                  </div>
                ) : (
                  <div className="text-sm text-wk-mist">
                    No snapshots yet.{" "}
                    <Link to={`/servers/${id}/saves`} className="text-wk-brasshi hover:text-wk-parchment">
                      Take the first →
                    </Link>
                  </div>
                )
              ) : (
                <div className="text-sm text-wk-mist">Save snapshots are steward-only.</div>
              )}
            </WkPanel>

            <WkPanel title="Recent activity" meta="last 24h">
              {activityQuery.data?.events.length ? (
                <ul className="space-y-1.5 text-sm">
                  {activityQuery.data.events.slice(0, 6).map((e) => (
                    <li key={e.id} className="flex items-baseline justify-between gap-2">
                      <span>
                        <span
                          className={
                            e.event === "join"
                              ? "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-wk-ok"
                              : "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-[#3a4148]"
                          }
                        />
                        <b className="font-bold text-wk-parchment">{e.name}</b>{" "}
                        <span className="text-wk-mist">{e.event === "join" ? "joined" : "left"}</span>
                      </span>
                      <span className="whitespace-nowrap text-xs text-wk-mist">{agoLabel(e.ts)}</span>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className="text-sm text-wk-mist">Quiet on the walls — no joins or leaves in 24 hours.</div>
              )}
            </WkPanel>
          </div>
        </div>
      </div>
    </div>
  );
}
