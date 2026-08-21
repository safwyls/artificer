import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "../../lib/api";
import { WkNote, WkPanel } from "./WkPanel";

/**
 * Companion sharing: the admin-facing half of the Artificer Companion
 * app. The console reads a character's sheet from the world save while
 * that player is connected and remembers it afterwards, so sharing is no
 * longer the only path — it is how a sheet stays current for someone who
 * has not logged in, and how a player gets their own local view. This
 * panel mints the token that makes that choice possible, and re-enabling
 * mints a fresh one (revoking every copy players hold).
 */
export function WkCompanionPanel({ serverId }: { serverId: number }) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["companion", serverId],
    queryFn: () => api.getCompanion(serverId),
  });
  const update = useMutation({
    mutationFn: (enabled: boolean) => api.setCompanion(serverId, enabled),
    onSuccess: (_data, enabled) => {
      toast.success(enabled ? "Companion sharing enabled" : "Companion sharing disabled");
      queryClient.invalidateQueries({ queryKey: ["companion", serverId] });
      queryClient.invalidateQueries({ queryKey: ["world", serverId] });
    },
    onError: (e) => toast.error("Update failed", { description: errorDetail(e) ?? (e as Error).message }),
  });

  const state = query.data;
  return (
    <WkPanel
      title="Companion sharing"
      meta={state?.enabled ? `${state.shared ?? 0} character${(state.shared ?? 0) === 1 ? "" : "s"} shared` : "off"}
    >
      <p className="text-sm text-wk-mist">
        This console reads a character's sheet from the world save while that player is connected, and
        remembers it afterwards. Sharing covers the rest: a player running the old{" "}
        <b className="text-wk-parchment">wkcompanion</b> relay keeps their sheet current without logging in.
        The current Artificer Companion focuses on world save sync and no longer relays character data, so
        this tier matters mainly for players still running the older app.
      </p>
      {state?.enabled && (
        <>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <code className="rounded-sm bg-wk-ink px-3 py-1.5 font-mono text-xs tracking-[0.06em] text-wk-parchment">
              {state.token}
            </code>
            <button
              onClick={() => {
                navigator.clipboard.writeText(state.token);
                toast.success("Token copied");
              }}
              className="rounded-sm border border-wk-edge px-2.5 py-1 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
            >
              Copy token
            </button>
            <a
              href={`/api/public/companion/${state.token}/download`}
              className="rounded-sm border border-wk-edge px-2.5 py-1 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
            >
              Download app (Windows)
            </a>
            <button
              onClick={() => {
                navigator.clipboard.writeText(
                  `${window.location.origin}/api/public/companion/${state.token}/download`,
                );
                toast.success("Download link copied");
              }}
              className="rounded-sm border border-wk-edge px-2.5 py-1 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
            >
              Copy download link
            </button>
          </div>
          <p className="mt-2 text-xs text-wk-mist">
            The download serves a relay-capable build only when this deployment bundles one (COMPANION_EXE);
            players already running wkcompanion keep working with this console's address and the token.
          </p>
        </>
      )}
      <div className="mt-3">
        <button
          onClick={() => update.mutate(!state?.enabled)}
          disabled={update.isPending || query.isLoading}
          className="rounded border border-wk-brass bg-gradient-to-b from-[#2a2416] to-[#1e1a10] px-4 py-1.5 font-bold tracking-[0.05em] text-wk-brasshi transition hover:brightness-125 disabled:opacity-50"
        >
          {state?.enabled ? "Disable sharing" : "Enable sharing"}
        </button>
      </div>
      <WkNote>
        The token is the whole credential — anyone holding it can push character data here, so share it like a
        password. Disabling revokes it and forgets every sheet it delivered, including copies this console had
        folded into memory; sheets it read from the world save itself are its own and stay. Re-enabling mints a
        fresh token, and shared sheets re-arrive within minutes of a console restart while players run the app.
      </WkNote>
    </WkPanel>
  );
}
