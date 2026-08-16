import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Ban, type Player } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { FtBans } from "../../components/flametender/FtBans";
import { FtNote, FtPanel } from "../../components/flametender/FtPanel";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../../components/ui/dialog";

/**
 * Kick and Ban are rendered, disabled, with the reason — the established
 * pattern here. Hiding them would leave stewards wondering where
 * moderation went; a dead button that 502s would be a lie in the other
 * direction.
 *
 * Kick stays dead: there is genuinely no kick outside the in-game player
 * menu. Ban does not, because there *is* something real to do — write the
 * account to the list the game reads at start. So it becomes live for
 * moderators under a label that doesn't overpromise ("Add to ban list"),
 * with a confirm that says plainly it ejects nobody now.
 */
const KICK_REASON = "Kick from the in-game player menu — the server has no admin API";
const BAN_REASON = "Ban from the in-game player menu; bans persist in enshrouded_server.json";

/** Ban action for one roster row. Absent for anyone without the moderation
 * grant, and inert for an account already on the list. */
function BanPlayerButton({ serverId, player }: { serverId: number; player: Player }) {
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);

  const bansQuery = useQuery({
    queryKey: ["server-bans", serverId],
    queryFn: () => api.serverBans(serverId),
    retry: false,
  });
  const save = useMutation({
    mutationFn: (bans: Ban[]) => api.updateServerBans(serverId, bans),
    onSuccess: (res) => {
      queryClient.setQueryData(["server-bans", serverId], res);
      setConfirming(false);
      toast.success(`${player.name} added to the ban list`, {
        description: "It takes effect at the next restart — they are still in the world now.",
      });
    },
    onError: (e: Error) => toast.error("Ban failed", { description: e.message }),
  });

  // The roster's userId is the SteamID64 (the join line carries it); when
  // even that never arrived it falls back to a peer handle, which is not
  // something that can be banned.
  const id = player.userId;
  const bannable = /^\d{15,20}$/.test(id);
  if (!bansQuery.data || !bannable) return null;

  const already = bansQuery.data.bans.some((b) => b.id === id);
  if (already) {
    return (
      <span
        title="Already on the ban list"
        className="ml-1.5 rounded-sm border border-ft-sporedim px-2.5 py-0.5 text-xs text-ft-spore"
      >
        Banned
      </span>
    );
  }

  return (
    <>
      <button
        onClick={() => setConfirming(true)}
        className="ml-1.5 rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen transition hover:border-ft-sporedim hover:text-ft-spore"
      >
        Add to ban list
      </button>
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent className="flametender border-ft-edge bg-ft-panel font-ftbody text-ft-bone">
          <DialogHeader>
            <DialogTitle className="font-ftdisplay tracking-[0.06em] text-ft-stonehi">
              Ban {player.name}?
            </DialogTitle>
            <DialogDescription className="text-ft-lichen">
              This writes <code className="font-mono text-ft-flame">{id}</code> to the banned list in
              enshrouded_server.json. It does not remove them now — the game reads that list when it starts, so the ban
              holds from the next restart. To eject someone immediately, use the in-game player menu with a kick/ban
              role password.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              onClick={() => setConfirming(false)}
              className="rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:text-ft-bone"
            >
              Cancel
            </button>
            <button
              onClick={() => save.mutate([...bansQuery.data.bans, { index: -1, id }])}
              disabled={save.isPending}
              className="rounded border border-ft-sporedim bg-ft-sporedim/30 px-4 py-1.5 text-sm font-bold text-ft-spore transition hover:brightness-125 disabled:opacity-50"
            >
              {save.isPending ? "Saving…" : "Add to ban list"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

/** The player table rows, shared by the overview panel and this page. */
export function FtPlayerRows({
  serverId,
  players,
  online,
  loading,
}: {
  serverId: number;
  players: Player[];
  online: boolean;
  loading: boolean;
}) {
  const { can } = useAuth();
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
              {/* Usually the player's own name; the SteamID64 when the
                  login line that carries it scrolled past unseen. Mono
                  either way, since one of the two forms is an id. */}
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
              {can("moderate") ? (
                <BanPlayerButton serverId={serverId} player={p} />
              ) : (
                <button
                  disabled
                  title={BAN_REASON}
                  className="ml-1.5 cursor-not-allowed rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen opacity-40"
                >
                  Ban
                </button>
              )}
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
  const { can } = useAuth();

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
          meta="derived from the server log · name and Steam ID per join"
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
            raise, rotatable in Configuration) unlocks kick and ban in the in-game player menu. Bans made there persist
            to the same list this page edits.
          </FtNote>
        </FtPanel>

        {can("moderate") && <FtBans serverId={id} />}

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
