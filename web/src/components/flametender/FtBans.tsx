import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Ban } from "../../lib/api";
import { FtNote, FtPanel } from "./FtPanel";

/**
 * The ban list (docs/design.md, "The moderation surface").
 *
 * `bannedAccounts` is where the in-game ban UI persists its bans, and the
 * only ban surface this console can reach — Enshrouded has no RCON and no
 * admin API. So this panel is honest about its half of the job: it edits
 * the list the game reads at start. It ejects nobody, and it says so.
 *
 * It lives here, on Flameborn, under the moderation grant rather than the
 * settings grant: unlike the role groups, this list carries no passwords,
 * and a moderator should be able to lift a ban without being handed every
 * credential on the server.
 */

/** The id shape the console can check for: a SteamID64 is 17 digits. The
 * check is deliberately loose (the backend runs the same one) because the
 * id space is still an open question in the recon doc. */
function plausibleId(id: string): boolean {
  return /^\d{15,20}$/.test(id.trim());
}

export function FtBans({ serverId }: { serverId: number }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Ban[] | null>(null);
  const [entry, setEntry] = useState("");

  const bansQuery = useQuery({
    queryKey: ["server-bans", serverId],
    queryFn: () => api.serverBans(serverId),
    retry: false,
  });

  const save = useMutation({
    mutationFn: (bans: Ban[]) => api.updateServerBans(serverId, bans),
    onSuccess: (res) => {
      toast.success("Ban list saved — applies at the next restart");
      setDraft(null);
      queryClient.setQueryData(["server-bans", serverId], res);
    },
    onError: (e: Error) => toast.error("Save failed", { description: e.message }),
  });

  const saved = bansQuery.data?.bans;
  const bans = draft ?? saved ?? [];
  const dirty = useMemo(
    () => draft !== null && JSON.stringify(draft) !== JSON.stringify(saved ?? []),
    [draft, saved],
  );

  const add = () => {
    const id = entry.trim();
    if (!plausibleId(id)) {
      toast.error("That doesn't look like a SteamID64", {
        description: "It's the player's 17-digit account id — the roster shows it when no name has been seen yet.",
      });
      return;
    }
    if (bans.some((b) => b.id === id)) {
      toast.error("Already on the list");
      return;
    }
    setDraft([...bans, { index: -1, id }]);
    setEntry("");
  };

  if (bansQuery.error instanceof ApiError && bansQuery.error.status === 400) {
    // No config path: the file this list lives in isn't reachable. The
    // Configuration page is where that gets fixed, and says so there.
    return null;
  }

  const data = bansQuery.data;

  return (
    <FtPanel title="Banned accounts" meta={data ? `${bans.length} listed` : undefined}>
      {bansQuery.isLoading && <p className="text-sm text-ft-lichen">Reading the file…</p>}
      {bansQuery.isError && <p className="text-sm text-ft-spore">{(bansQuery.error as Error).message}</p>}
      {data && (
        <>
          {data.running && (
            <p className="mb-2.5 rounded-sm border border-ft-sporedim bg-ft-sporedim/20 px-2.5 py-1.5 text-xs text-ft-spore">
              The server is running, and it owns this list while it is — banning from the in-game menu writes the same
              file. An edit made now can be overwritten when the game next saves it. Stop the server first for a change
              that's certain to stick.
            </p>
          )}
          {!data.writable && (
            <p className="mb-2 text-xs text-ft-spore">
              The file is on a read-only mount — saving will fail until it's mounted read-write.
            </p>
          )}

          <div className="flex items-center gap-2">
            <input
              value={entry}
              onChange={(e) => setEntry(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && add()}
              placeholder="SteamID64"
              aria-label="Account to ban"
              spellCheck={false}
              className="w-56 rounded-sm border border-ft-edge bg-ft-void px-2 py-1 font-mono text-xs text-ft-bone outline-none focus:border-ft-flamedim"
            />
            <button
              onClick={add}
              className="flex items-center gap-1.5 rounded border border-ft-edge px-3 py-1 text-sm text-ft-lichen transition hover:border-ft-stone hover:text-ft-stonehi"
            >
              <Plus className="h-3.5 w-3.5" /> Add
            </button>
          </div>

          {bans.length === 0 ? (
            <p className="mt-3 text-sm text-ft-lichen">Nobody is banned.</p>
          ) : (
            <ul className="mt-2">
              {bans.map((b, i) => (
                <li
                  key={`${b.index}-${b.id}`}
                  className="flex items-center justify-between gap-2 border-t border-ft-edge py-2"
                >
                  <span className="min-w-0">
                    <span className="font-mono text-[13px] text-ft-bone">{b.id}</span>
                    {b.name && <span className="ml-2 text-sm text-ft-lichen">{b.name}</span>}
                  </span>
                  <button
                    onClick={() => setDraft(bans.filter((_, j) => j !== i))}
                    className="rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen transition hover:border-ft-sporedim hover:text-ft-spore"
                  >
                    Lift
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-ft-edge pt-3">
            <button
              onClick={() => save.mutate(bans)}
              disabled={!dirty || save.isPending}
              className="rounded border border-ft-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-ft-stonehi transition hover:brightness-125 disabled:opacity-40"
            >
              {save.isPending ? "Saving…" : "Save ban list"}
            </button>
            {dirty && (
              <button
                onClick={() => setDraft(null)}
                className="rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:text-ft-bone"
              >
                Discard
              </button>
            )}
          </div>

          {data.unreadable > 0 && (
            <p className="mt-2 text-xs text-ft-spore">
              {data.unreadable} {data.unreadable === 1 ? "entry" : "entries"} in this file aren't in a form the console
              recognises. They're left exactly as they are — saving here won't lift them.
            </p>
          )}
          <FtNote>
            The game reads this list when it starts, so a ban added here takes effect at the next restart and does not
            remove anyone who is in the world now. To eject someone immediately, join with a kick/ban role password and
            use the in-game player menu — bans made there land in this same list.
          </FtNote>
        </>
      )}
    </FtPanel>
  );
}
