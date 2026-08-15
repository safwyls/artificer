import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Eye, EyeOff } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type ConfigSetting } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { copyText } from "../../lib/utils";
import { FtNote, FtPanel } from "../../components/flametender/FtPanel";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "../../components/ui/dialog";

const SECRET_KEYS = new Set(["AdminPassword", "WorldPassword"]);

function SettingRow({
  setting,
  draft,
  onChange,
}: {
  setting: ConfigSetting;
  draft: string | undefined;
  onChange: (value: string) => void;
}) {
  const [revealed, setRevealed] = useState(false);
  const secret = SECRET_KEYS.has(setting.key);
  const value = draft ?? setting.value;
  const dirty = draft !== undefined && draft !== setting.value;

  return (
    <div className="flex items-center gap-2.5 border-t border-ft-edge py-2 first:border-t-0">
      <span className="w-44 shrink-0 font-mono text-xs text-ft-flame">{setting.key}</span>
      {setting.type === "bool" ? (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="rounded-sm border border-ft-edge bg-ft-void px-2 py-1 font-mono text-xs text-ft-bone"
        >
          <option value="True">True</option>
          <option value="False">False</option>
        </select>
      ) : (
        <input
          type={secret && !revealed ? "password" : "text"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          spellCheck={false}
          className="min-w-0 flex-1 rounded-sm border border-ft-edge bg-ft-void px-2 py-1 font-mono text-xs text-ft-bone outline-none focus:border-ft-flamedim"
        />
      )}
      {secret && (
        <button
          onClick={() => setRevealed((r) => !r)}
          title={revealed ? "Hide value" : "Reveal value"}
          className="text-ft-lichen transition hover:text-ft-bone"
        >
          {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
        </button>
      )}
      {dirty && <span className="text-[10px] uppercase tracking-[0.08em] text-ft-stonehi">edited</span>}
    </div>
  );
}

export function FtConfig() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { can, isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [rotated, setRotated] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const configQuery = useQuery({
    queryKey: ["server-config", id],
    queryFn: () => api.serverConfig(id),
    enabled: can("settings"),
    retry: false,
  });

  const save = useMutation({
    mutationFn: (changes: Record<string, string>) => api.updateServerConfig(id, changes),
    onSuccess: () => {
      toast.success("Config saved — restart to apply");
      setDrafts({});
      queryClient.invalidateQueries({ queryKey: ["server-config", id] });
    },
    onError: (e: Error) => toast.error(e.message || "Save failed"),
  });
  const rotate = useMutation({
    mutationFn: () => api.rotateAdminPassword(id),
    onSuccess: (res) => {
      setRotated(res.password);
      queryClient.invalidateQueries({ queryKey: ["server-config", id] });
    },
    onError: (e: Error) => toast.error(e.message || "Rotation failed"),
  });

  if (!can("settings")) {
    return (
      <div className="flametender min-h-full font-ftbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <FtPanel title="enshrouded_server.json">
            <p className="text-sm text-ft-lichen">
              The config file holds the role passwords in the clear, so reading it needs the settings permission.
            </p>
          </FtPanel>
        </div>
      </div>
    );
  }

  const config = configQuery.data;
  const noPath = configQuery.error instanceof ApiError && configQuery.error.status === 400;
  const changes = Object.fromEntries(
    Object.entries(drafts).filter(([k, v]) => config?.settings.find((s) => s.key === k)?.value !== v),
  );
  const dirtyCount = Object.keys(changes).length;

  return (
    <div className="flametender min-h-full font-ftbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        <FtPanel title="enshrouded_server.json" meta={config ? config.path : "restart to apply"}>
          {configQuery.isLoading && <p className="text-sm text-ft-lichen">Reading the file…</p>}
          {noPath && (
            <p className="text-sm text-ft-lichen">
              No config path is set for this server. Point Settings → Config path at{" "}
              <code className="font-mono text-xs text-ft-flame">enshrouded_server.json</code> in the install root (or
              the folder holding it) to edit it here.
            </p>
          )}
          {configQuery.isError && !noPath && (
            <p className="text-sm text-ft-spore">{(configQuery.error as Error).message}</p>
          )}
          {config && (
            <>
              {!config.writable && (
                <p className="mb-2 text-xs text-ft-spore">
                  The file is on a read-only mount — edits here will fail until it's mounted read-write.
                </p>
              )}
              <div>
                {config.settings.map((s) => (
                  <SettingRow
                    key={s.key}
                    setting={s}
                    draft={drafts[s.key]}
                    onChange={(v) => setDrafts((d) => ({ ...d, [s.key]: v }))}
                  />
                ))}
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-ft-edge pt-3">
                <button
                  onClick={() => save.mutate(changes)}
                  disabled={dirtyCount === 0 || save.isPending}
                  className="rounded border border-ft-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-ft-stonehi transition hover:brightness-125 disabled:opacity-40"
                >
                  {save.isPending ? "Saving…" : dirtyCount > 0 ? `Save ${dirtyCount} change${dirtyCount > 1 ? "s" : ""}` : "Save changes"}
                </button>
                {dirtyCount > 0 && (
                  <button
                    onClick={() => setDrafts({})}
                    className="rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:text-ft-bone"
                  >
                    Discard
                  </button>
                )}
                {isAdmin && (
                  <button
                    onClick={() => rotate.mutate()}
                    disabled={rotate.isPending}
                    className="ml-auto rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:border-ft-stone hover:text-ft-stonehi disabled:opacity-50"
                  >
                    {rotate.isPending ? "Rotating…" : "Rotate admin password"}
                  </button>
                )}
              </div>
              <FtNote>
                Edits apply on the next restart. Only existing keys can change, values are checked against their
                current type, and the previous file is kept one level deep as{" "}
                <code className="font-mono not-italic text-ft-flame">.bak</code>. Role passwords and difficulty presets
                beyond these scalars are Phase 2's role editor — until then the gameSettings block only applies when{" "}
                <code className="font-mono not-italic text-ft-flame">gameSettingsPreset</code> is{" "}
                <code className="font-mono not-italic text-ft-flame">Custom</code>.
              </FtNote>
            </>
          )}
        </FtPanel>
      </div>

      <Dialog open={rotated !== null} onOpenChange={(open) => !open && setRotated(null)}>
        <DialogContent className="flametender border-ft-edge bg-ft-panel font-ftbody text-ft-bone">
          <DialogHeader>
            <DialogTitle className="font-ftdisplay tracking-[0.06em] text-ft-stonehi">
              Admin password rotated
            </DialogTitle>
            <DialogDescription className="text-ft-lichen">
              This is the only time the new password is shown. It gates the next join — whoever should hold kick/ban
              types it at the join screen; previous holders lose the role when they next rejoin.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded bg-ft-void px-3 py-2.5">
            <code className="flex-1 font-mono text-sm text-ft-bone">{rotated}</code>
            <button
              onClick={async () => {
                if (rotated && (await copyText(rotated))) {
                  setCopied(true);
                  setTimeout(() => setCopied(false), 2000);
                }
              }}
              title="Copy password"
              className="text-ft-lichen transition hover:text-ft-bone"
            >
              {copied ? <Check className="h-4 w-4 text-ft-ok" /> : <Copy className="h-4 w-4" />}
            </button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
