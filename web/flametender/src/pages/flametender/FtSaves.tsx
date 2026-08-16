import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { FtNote, FtPanel } from "../../components/flametender/FtPanel";

function sizeLabel(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function FtSaves() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const backupsQuery = useQuery({
    queryKey: ["backups", id],
    queryFn: () => api.listBackups(id),
    enabled: isAdmin,
    // Close while one is running, idle otherwise. A snapshot takes seconds
    // and its outcome is the thing being waited for; a 30s poll left the
    // button reading "in progress" long after it had finished or failed.
    refetchInterval: (q) => (q.state.data?.running ? 2_000 : 30_000),
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
      <div className="flametender min-h-full font-ftbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <FtPanel title="World saves">
            <p className="text-sm text-ft-lichen">
              Save snapshots hold the whole world, so only stewards with the admin role can see them.
            </p>
          </FtPanel>
        </div>
      </div>
    );
  }

  const backups = backupsQuery.data;

  return (
    <div className="flametender min-h-full font-ftbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        <FtPanel
          title="World saves"
          meta={
            backups
              ? `${backups.snapshots.length} kept · ${sizeLabel(backups.totalBytes)}${
                  backups.intervalHours ? ` · every ${backups.intervalHours}h` : " · no schedule"
                }`
              : undefined
          }
        >
          {backupsQuery.isLoading && <p className="text-sm text-ft-lichen">Reading the vault…</p>}
          {backups && !backups.available && (
            <p className="text-sm text-ft-lichen">
              This server has no save path configured, so there is nothing to snapshot. Set one in Settings.
            </p>
          )}
          {backups?.available && (
            <>
              <div className="mb-3 flex flex-wrap items-center gap-2">
                <button
                  onClick={() => run.mutate()}
                  disabled={run.isPending || backups.running}
                  className="rounded border border-ft-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 font-bold tracking-[0.05em] text-ft-stonehi transition hover:brightness-125 disabled:opacity-50"
                >
                  {backups.running ? "Snapshot in progress…" : "Take snapshot now"}
                </button>
                <span className="text-xs text-ft-lichen">
                  The game saves every 10 minutes and on shutdown, keeping ~10 rolling copies of its own — a snapshot
                  archives the whole set, rollback window included.
                </span>
              </div>
              {/* A snapshot runs detached from the click that starts it, so a
                  failure has no request left to fail. Spore, because this is
                  the palette's one "something is wrong" colour. */}
              {backups.lastFailure && !backups.running && (
                <p className="mb-3 rounded-sm border border-ft-sporedim bg-ft-sporedim/20 px-2.5 py-1.5 text-xs text-ft-spore">
                  The last snapshot failed {new Date(backups.lastFailure.at).toLocaleString()} and wrote no file:{" "}
                  {backups.lastFailure.error}
                </p>
              )}
              {backups.snapshots.length === 0 ? (
                <p className="text-sm text-ft-lichen">The vault is empty — take the first snapshot.</p>
              ) : (
                <div>
                  {backups.snapshots.map((s) => (
                    <div
                      key={s.name}
                      className="flex items-center justify-between gap-2.5 border-t border-ft-edge py-2.5 first:border-t-0 first:pt-0"
                    >
                      <div>
                        <span className="font-mono text-xs text-ft-bone">{s.name}</span>
                        <br />
                        <span className="text-xs text-ft-lichen">
                          {new Date(s.ts).toLocaleString()} · {sizeLabel(s.bytes)}
                        </span>
                      </div>
                      <div className="flex gap-1.5">
                        <a
                          href={api.backupDownloadURL(id, s.name)}
                          className="rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen transition hover:border-ft-stone hover:text-ft-stonehi"
                        >
                          Download
                        </a>
                        <button
                          onClick={() => {
                            if (confirm(`Delete snapshot ${s.name}? This cannot be undone.`)) remove.mutate(s.name);
                          }}
                          className="rounded-sm border border-ft-edge px-2.5 py-0.5 text-xs text-ft-lichen transition hover:border-ft-sporedim hover:text-ft-spore"
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
          <FtNote>
            The world lives in savegame/ as a hex-named file plus rolling copies and an index that picks which copy
            loads. In-place restore and rollback land with the save-index reader (roadmap Phase 3) — until then,
            download a snapshot and place it by hand while the server is stopped.
          </FtNote>
        </FtPanel>
      </div>
    </div>
  );
}
