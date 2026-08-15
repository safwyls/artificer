import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { FkNote, FkPanel } from "../../components/flamekeeper/FkPanel";

function sizeLabel(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function FkSaves() {
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
      <div className="flamekeeper min-h-full font-fkbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <FkPanel title="World saves">
            <p className="text-sm text-fk-lichen">
              Save snapshots hold the whole world, so only stewards with the admin role can see them.
            </p>
          </FkPanel>
        </div>
      </div>
    );
  }

  const backups = backupsQuery.data;

  return (
    <div className="flamekeeper min-h-full font-fkbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        <FkPanel
          title="World saves"
          meta={
            backups
              ? `${backups.snapshots.length} kept · ${sizeLabel(backups.totalBytes)}${
                  backups.intervalHours ? ` · every ${backups.intervalHours}h` : " · no schedule"
                }`
              : undefined
          }
        >
          {backupsQuery.isLoading && <p className="text-sm text-fk-lichen">Reading the vault…</p>}
          {backups && !backups.available && (
            <p className="text-sm text-fk-lichen">
              This server has no save path configured, so there is nothing to snapshot. Set one in Settings.
            </p>
          )}
          {backups?.available && (
            <>
              <div className="mb-3 flex flex-wrap items-center gap-2">
                <button
                  onClick={() => run.mutate()}
                  disabled={run.isPending || backups.running}
                  className="rounded border border-fk-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 font-bold tracking-[0.05em] text-fk-stonehi transition hover:brightness-125 disabled:opacity-50"
                >
                  {backups.running ? "Snapshot in progress…" : "Take snapshot now"}
                </button>
                <span className="text-xs text-fk-lichen">
                  The game saves every 10 minutes and on shutdown, keeping ~10 rolling copies of its own — a snapshot
                  archives the whole set, rollback window included.
                </span>
              </div>
              {backups.snapshots.length === 0 ? (
                <p className="text-sm text-fk-lichen">The vault is empty — take the first snapshot.</p>
              ) : (
                <div>
                  {backups.snapshots.map((s) => (
                    <div
                      key={s.name}
                      className="flex items-center justify-between gap-2.5 border-t border-fk-edge py-2.5 first:border-t-0 first:pt-0"
                    >
                      <div>
                        <span className="font-mono text-xs text-fk-bone">{s.name}</span>
                        <br />
                        <span className="text-xs text-fk-lichen">
                          {new Date(s.ts).toLocaleString()} · {sizeLabel(s.bytes)}
                        </span>
                      </div>
                      <div className="flex gap-1.5">
                        <a
                          href={api.backupDownloadURL(id, s.name)}
                          className="rounded-sm border border-fk-edge px-2.5 py-0.5 text-xs text-fk-lichen transition hover:border-fk-stone hover:text-fk-stonehi"
                        >
                          Download
                        </a>
                        <button
                          onClick={() => {
                            if (confirm(`Delete snapshot ${s.name}? This cannot be undone.`)) remove.mutate(s.name);
                          }}
                          className="rounded-sm border border-fk-edge px-2.5 py-0.5 text-xs text-fk-lichen transition hover:border-fk-sporedim hover:text-fk-spore"
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
          <FkNote>
            The world lives in savegame/ as a hex-named file plus rolling copies and an index that picks which copy
            loads. In-place restore and rollback land with the save-index reader (roadmap Phase 3) — until then,
            download a snapshot and place it by hand while the server is stopped.
          </FkNote>
        </FkPanel>
      </div>
    </div>
  );
}
