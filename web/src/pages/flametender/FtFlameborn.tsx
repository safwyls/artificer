import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type Player } from "../../lib/api";
import { FtNote, FtPanel } from "../../components/flametender/FtPanel";

/**
 * Kick and Ban are rendered, disabled, with the reason — the established
 * pattern here. Hiding them would leave stewards wondering where
 * moderation went; a dead button that 502s would be a lie in the other
 * direction. For Enshrouded they stay disabled by design: moderation
 * lives in the in-game player menu, and the reason says so.
 */
const KICK_REASON = "Kick from the in-game player menu — the server has no admin API";
const BAN_REASON = "Ban from the in-game player menu; bans persist in enshrouded_server.json";

/** The player table rows, shared by the overview panel and this page. */
export function FtPlayerRows({
  players,
  online,
  loading,
}: {
  serverId: number;
  players: Player[];
  online: boolean;
  loading: boolean;
}) {
  if (loading) return <p className="py-3 text-sm text-ft-lichen">Reading the log…</p>;
  if (!online) return <p className="py-3 text-sm text-ft-lichen">The server is offline — nobody is in the world.</p>;
  if (players.length === 0)
    return <p className="py-3 text-sm text-ft-lichen">The fire burns alone — no Flameborn online.</p>;
  return (
    <table className="w-full border-collapse text-sm">
      <thead>
        <tr>
          <th className="px-2.5 pb-2 pt-1 text-left text-[11px] font-medium uppercase tracking-[0.12em] text-ft-lichen">
            Name
          </th>
          <th className="px-2.5 pb-2 pt-1 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-ft-lichen" />
        </tr>
      </thead>
      <tbody>
        {players.map((p) => (
          <tr key={p.userId}>
            <td className="border-t border-ft-edge px-2.5 py-2.5">
              <span className="mr-2 inline-block h-[7px] w-[7px] rounded-full bg-ft-flame shadow-[0_0_5px_rgba(127,195,240,.6)]" />
              {/* The log carries SteamID64s only, so the id doubles as the
                  name until the A2S query (roadmap Phase 2) brings real
                  ones. Mono, because it is an identifier being honest. */}
              <span className="font-mono text-[13px] font-bold text-ft-bone">{p.name}</span>
            </td>
            <td className="border-t border-ft-edge px-2.5 py-2.5 text-right">
              <button
                disabled
                title={KICK_REASON}
                className="cursor-not-allowed rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen opacity-40"
              >
                Kick
              </button>
              <button
                disabled
                title={BAN_REASON}
                className="ml-1.5 cursor-not-allowed rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen opacity-40"
              >
                Ban
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function FtFlameborn() {
  const { serverID } = useParams();
  const id = Number(serverID);

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
  const activityQuery = useQuery({
    queryKey: ["server-activity", id, 168],
    queryFn: () => api.serverActivity(id, 168),
  });

  const online = !infoQuery.isError && !!infoQuery.data;

  return (
    <div className="flametender min-h-full font-ftbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        <FtPanel
          title="Flameborn"
          meta="derived from the server log · Steam IDs until the A2S query lands"
          bodyClassName="pt-1.5"
        >
          <FtPlayerRows
            serverId={id}
            players={playersQuery.data ?? []}
            online={online}
            loading={playersQuery.isLoading}
          />
          <FtNote>
            Moderation is role-based: joining with a kick/ban-capable role password (the admin password from the
            raise, rotatable in Configuration) unlocks kick and ban in the in-game player menu. Bans persist to the
            config's banned list.
          </FtNote>
        </FtPanel>

        <FtPanel title="Comings and goings" meta="last 7 days">
          {activityQuery.isLoading && <p className="text-sm text-ft-lichen">Loading history…</p>}
          {activityQuery.data &&
            (activityQuery.data.events.length === 0 ? (
              <p className="text-sm text-ft-lichen">No joins or leaves recorded this week.</p>
            ) : (
              <ul className="space-y-1.5 text-sm">
                {activityQuery.data.events.slice(0, 40).map((e) => (
                  <li key={e.id} className="flex items-baseline justify-between gap-2 border-t border-ft-edge pt-1.5 first:border-t-0 first:pt-0">
                    <span>
                      <span
                        className={
                          e.event === "join"
                            ? "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-ft-ok"
                            : "mr-2 inline-block h-[7px] w-[7px] rounded-full bg-[#3a4148]"
                        }
                      />
                      <b className="font-bold text-ft-bone">{e.name}</b>{" "}
                      <span className="text-ft-lichen">{e.event === "join" ? "joined the world" : "left the world"}</span>
                    </span>
                    <span className="whitespace-nowrap font-mono text-xs text-ft-lichen">
                      {new Date(e.ts).toLocaleString()}
                    </span>
                  </li>
                ))}
              </ul>
            ))}
        </FtPanel>
      </div>
    </div>
  );
}
