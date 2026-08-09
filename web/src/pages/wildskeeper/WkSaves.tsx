import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { WkNote, WkPanel } from "../../components/wildskeeper/WkPanel";

function sizeLabel(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function WkSaves() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();

  const backupsQuery = useQuery({
    queryKey: ["backups", id],
    queryFn: () => api.listBackups(id),
    enabled: isAdmin,
    refetchInterval: 30_000,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["backups", id] });
  const run = useMutation({
    mutationFn: () => api.runBackup(id),
    onSuccess: () => {
      toast.success("Snapshot started");
      invalidate();
    },
    onError: (e: Error) => toast.error(e.message || "Snapshot failed to start"),
  });
  const remove = useMutation({
    mutationFn: (name: string) => api.deleteBackup(id, name),
    onSuccess: () => {
      toast.success("Snapshot deleted");
      invalidate();
    },
    onError: (e: Error) => toast.error(e.message || "Delete failed"),
  });

  if (!isAdmin) {
    return (
      <div className="wildskeeper min-h-full font-wkbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <WkPanel title="World saves">
            <p className="text-sm text-wk-mist">
              Save snapshots hold the whole world, so only stewards with the admin role can see them.
            </p>
          </WkPanel>
        </div>
      </div>
    );
  }

  const backups = backupsQuery.data;

  return (
    <div className="wildskeeper min-h-full font-wkbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        <WkPanel
          title="World saves"
          meta={
            backups
              ? `${backups.snapshots.length} kept · ${sizeLabel(backups.totalBytes)}${
                  backups.intervalHours ? ` · every ${backups.intervalHours}h` : " · no schedule"
                }`
              : undefined
          }
        >
          {backupsQuery.isLoading && <p className="text-sm text-wk-mist">Reading the vault…</p>}
          {backups && !backups.available && (
            <p className="text-sm text-wk-mist">
              This server has no save path configured, so there is nothing to snapshot. Set one in Settings.
            </p>
          )}
          {backups?.available && (
            <>
              <div className="mb-3 flex items-center gap-2">
                <button
                  onClick={() => run.mutate()}
                  disabled={run.isPending || backups.running}
                  className="rounded border border-wk-brass bg-gradient-to-b from-[#2a2416] to-[#1e1a10] px-4 py-1.5 font-bold tracking-[0.05em] text-wk-brasshi transition hover:brightness-125 disabled:opacity-50"
                >
                  {backups.running ? "Snapshot in progress…" : "Take snapshot now"}
                </button>
              </div>
              {backups.snapshots.length === 0 ? (
                <p className="text-sm text-wk-mist">The vault is empty — take the first snapshot.</p>
              ) : (
                <div>
                  {backups.snapshots.map((s) => (
                    <div
                      key={s.name}
                      className="flex items-center justify-between gap-2.5 border-t border-wk-edge py-2.5 first:border-t-0 first:pt-0"
                    >
                      <div>
                        <span className="font-mono text-xs text-wk-parchment">{s.name}</span>
                        <br />
                        <span className="text-xs text-wk-mist">
                          {new Date(s.ts).toLocaleString()} · {sizeLabel(s.bytes)}
                        </span>
                      </div>
                      <div className="flex gap-1.5">
                        <a
                          href={api.backupDownloadURL(id, s.name)}
                          className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
                        >
                          Download
                        </a>
                        <button
                          onClick={() => {
                            if (confirm(`Delete snapshot ${s.name}? This cannot be undone.`)) remove.mutate(s.name);
                          }}
                          className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-xs text-wk-mist transition hover:border-wk-emberdim hover:text-wk-ember"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
          <WkNote>
            The live server loads the newest .sav in SaveGames/ on start, and the filename must match the world name.
            Restoring a snapshot in place lands with the agent's supervisor work — until then, download and place it
            by hand while the server is stopped.
          </WkNote>
        </WkPanel>
      </div>
    </div>
  );
}
