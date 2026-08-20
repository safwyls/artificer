import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "../../lib/api";
import { WkNote, WkPanel } from "./WkPanel";

/**
 * Companion sharing: the admin-facing half of the wkcompanion app. The
 * game keeps each character's skills and inventory on the player's own
 * machine, so the console can only show them when players choose to share
 * — this panel mints the token that makes that choice possible, and
 * re-enabling mints a fresh one (revoking every copy players hold).
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
        The game keeps each adventurer's skills and inventory on their own computer — this server never sees
        them. Players who install the <b className="text-wk-parchment">wkcompanion</b> app can choose to share
        their character sheet with this console: hand them this console's address and the token below.
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
            Give players the download link — the console serves the app itself — plus this console's address
            and the token for the app's settings.
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
        password. Disabling revokes it and drops everything it delivered; re-enabling mints a fresh token.
        Shared sheets live in console memory and re-arrive within minutes of a restart while players run the
        app.
      </WkNote>
    </WkPanel>
  );
}
