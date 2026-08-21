import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  api,
  errorDetail,
  type SyncVersion,
  type SyncWorldSettings,
  type SyncWorldStatus,
} from "../lib/api";
import { useAuth } from "../lib/auth";
import { agoLabel } from "../lib/time";
import { WkNote, WkPanel } from "../components/wildskeeper/WkPanel";

/**
 * Shared-world custody (docs/save-sync-architecture.md): who holds each
 * world, its version history, and the custody verbs. Console-level, not
 * per-server — a world is a save with holders, and may only optionally
 * have a dedicated server behind it.
 */

function sizeLabel(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

const primaryBtn =
  "rounded border border-wk-brass bg-gradient-to-b from-[#2a2416] to-[#1e1a10] px-4 py-1.5 font-bold tracking-[0.05em] text-wk-brasshi transition hover:brightness-125 disabled:opacity-50";
const quietBtn =
  "rounded border border-wk-edge px-3 py-1.5 text-sm text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi disabled:opacity-50";
const dangerBtn =
  "rounded border border-wk-edge px-3 py-1.5 text-sm text-wk-mist transition hover:border-wk-emberdim hover:text-wk-ember disabled:opacity-50";

/** A hidden file input behind a button, for check-in and import uploads. */
function UploadButton({
  label,
  className,
  disabled,
  onFile,
}: {
  label: string;
  className: string;
  disabled?: boolean;
  onFile: (file: File) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept=".tar,application/x-tar"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) onFile(file);
          e.target.value = "";
        }}
      />
      <button className={className} disabled={disabled} onClick={() => inputRef.current?.click()}>
        {label}
      </button>
    </>
  );
}

function holderLine(status: SyncWorldStatus, myName: string | null): string {
  const h = status.holder;
  if (!h) return status.claimedBy ? `Free · next claim: ${status.claimedBy}` : "Free — nobody holds this world";
  const who = h.username === myName ? "you" : h.username;
  const server = h.serverHeld ? " (on the dedicated server)" : "";
  if (h.claimable) return `Held by ${who}${server} — the hold expired and is claimable`;
  return `Held by ${who}${server} until ${new Date(h.expiresAt).toLocaleString()}`;
}

function versionLabel(v: SyncVersion, uploaders: Record<string, string>): string {
  const by = v.uploaderId != null ? (uploaders[String(v.uploaderId)] ?? `user #${v.uploaderId}`) : "unknown";
  return `${v.kind} by ${by} · ${sizeLabel(v.bytes)} · ${agoLabel(v.createdAt)}`;
}

/** The version history for one expanded world. */
function WorldHistory({ world, headVersion }: { world: number; headVersion?: number }) {
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const detailQuery = useQuery({ queryKey: ["syncWorld", world], queryFn: () => api.getSyncWorld(world) });
  const setHead = useMutation({
    mutationFn: (versionId: number) => api.syncSetHead(world, versionId),
    onSuccess: () => {
      toast.success("Head moved");
      queryClient.invalidateQueries({ queryKey: ["syncWorlds"] });
      queryClient.invalidateQueries({ queryKey: ["syncWorld", world] });
    },
    onError: (e) => toast.error("Could not move the head", { description: errorDetail(e) }),
  });

  if (detailQuery.isLoading) return <p className="mt-3 text-sm text-wk-mist">Reading the ledger…</p>;
  const detail = detailQuery.data;
  if (!detail) return null;
  if (detail.versions.length === 0)
    return <p className="mt-3 text-sm text-wk-mist">No versions yet — the first check-in or import starts the history.</p>;

  return (
    <div className="mt-3 border-t border-wk-edge pt-1">
      {detail.versions.map((v) => (
        <div key={v.id} className="flex items-center justify-between gap-2.5 border-t border-wk-edge py-2 first:border-t-0">
          <div>
            <span className="font-mono text-xs text-wk-parchment">
              v{v.id}
              {headVersion === v.id && <span className="ml-2 rounded-sm bg-wk-ink px-1.5 py-0.5 text-[10px] text-wk-brasshi">head</span>}
              {v.conflict && (
                <span
                  className="ml-2 rounded-sm bg-wk-ink px-1.5 py-0.5 text-[10px] text-wk-ember"
                  title="Checked in from a hold that could no longer move the head. Kept until someone picks a head."
                >
                  conflict
                </span>
              )}
            </span>
            <br />
            <span className="text-xs text-wk-mist">{versionLabel(v, detail.uploaders)}</span>
          </div>
          <div className="flex gap-1.5">
            <a
              href={api.syncDownloadURL(world, v.id)}
              className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
            >
              Download
            </a>
            {isAdmin && headVersion !== v.id && (
              <button
                onClick={() => {
                  if (confirm(`Make v${v.id} the canonical head? The next checkout delivers it.`)) setHead.mutate(v.id);
                }}
                className="rounded-sm border border-wk-edge px-2.5 py-0.5 text-xs text-wk-mist transition hover:border-wk-brass hover:text-wk-brasshi"
              >
                Make head
              </button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

/** The admin settings form for one world, inline under its panel. */
function WorldSettings({ status, onDone }: { status: SyncWorldStatus; onDone: () => void }) {
  const w = status.world;
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SyncWorldSettings>({
    name: w.name,
    serverId: w.serverId ?? null,
    leaseHours: w.leaseHours,
    maxBytes: w.maxBytes,
    keepVersions: w.keepVersions,
    checkpoints: w.checkpoints,
    webhookUrl: w.webhookUrl,
  });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: api.listServers });
  const save = useMutation({
    mutationFn: () => api.updateSyncWorld(w.id, form),
    onSuccess: () => {
      toast.success("World settings saved");
      queryClient.invalidateQueries({ queryKey: ["syncWorlds"] });
      onDone();
    },
    onError: (e) => toast.error("Could not save settings", { description: errorDetail(e) }),
  });

  const field = "w-full rounded-sm border border-wk-edge bg-wk-ink px-2 py-1.5 text-sm text-wk-parchment";
  const label = "text-[11px] uppercase tracking-[0.14em] text-wk-mist";
  return (
    <div className="mt-3 grid gap-3 border-t border-wk-edge pt-3 sm:grid-cols-2">
      <div>
        <div className={label}>Name</div>
        <input className={field} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
      </div>
      <div>
        <div className={label} title="The dedicated server that can also hold this world">
          Linked server
        </div>
        <select
          className={field}
          value={form.serverId ?? ""}
          onChange={(e) => setForm({ ...form, serverId: e.target.value === "" ? null : Number(e.target.value) })}
        >
          <option value="">none — peer-hosted only</option>
          {(serversQuery.data ?? []).map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
      </div>
      <div>
        <div className={label} title="How long a checkout lasts before the hold becomes claimable">
          Lease (hours)
        </div>
        <input
          type="number"
          className={field}
          value={form.leaseHours}
          onChange={(e) => setForm({ ...form, leaseHours: Number(e.target.value) })}
        />
      </div>
      <div>
        <div className={label}>Keep versions</div>
        <input
          type="number"
          className={field}
          value={form.keepVersions}
          onChange={(e) => setForm({ ...form, keepVersions: Number(e.target.value) })}
        />
      </div>
      <div>
        <div className={label} title="Refuse uploads beyond this size">
          Max size (MiB)
        </div>
        <input
          type="number"
          className={field}
          value={Math.round(form.maxBytes / (1 << 20))}
          onChange={(e) => setForm({ ...form, maxBytes: Number(e.target.value) * (1 << 20) })}
        />
      </div>
      <div>
        <div className={label} title="Discord webhook for this world's custody events">
          Discord webhook URL
        </div>
        <input
          className={field}
          placeholder="https://discord.com/api/webhooks/…"
          value={form.webhookUrl}
          onChange={(e) => setForm({ ...form, webhookUrl: e.target.value })}
        />
      </div>
      <label className="flex items-center gap-2 text-sm text-wk-parchment">
        <input
          type="checkbox"
          checked={form.checkpoints}
          onChange={(e) => setForm({ ...form, checkpoints: e.target.checked })}
        />
        Accept mid-session checkpoints
        <span className="text-xs text-wk-mist">(crash insurance; costs the holder upstream bandwidth)</span>
      </label>
      <div className="flex items-end justify-end gap-2">
        <button className={quietBtn} onClick={onDone}>
          Cancel
        </button>
        <button className={primaryBtn} disabled={save.isPending} onClick={() => save.mutate()}>
          Save settings
        </button>
      </div>
    </div>
  );
}

function WorldPanel({ status }: { status: SyncWorldStatus }) {
  const w = status.world;
  const { username, isAdmin, can } = useAuth();
  const canSync = can("savesync");
  const queryClient = useQueryClient();
  const [showHistory, setShowHistory] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["syncWorlds"] });
    queryClient.invalidateQueries({ queryKey: ["syncWorld", w.id] });
  };
  const onError = (title: string) => (e: unknown) => toast.error(title, { description: errorDetail(e) });

  const checkout = useMutation({
    mutationFn: (takeover: boolean) => api.syncCheckout(w.id, takeover),
    onSuccess: () => {
      toast.success(`${w.name} is yours — download it below or let your companion app fetch it`);
      invalidate();
    },
    onError: onError("Checkout refused"),
  });
  const checkin = useMutation({
    mutationFn: (file: File) => api.syncCheckin(status.holder!.sessionId, file),
    onSuccess: (out) => {
      if (out.version.conflict) {
        toast.warning("Checked in, but flagged as a conflict — pick a head in the history");
      } else {
        toast.success("Checked in — the world is free");
      }
      invalidate();
    },
    onError: onError("Check-in refused"),
  });
  const claim = useMutation({
    mutationFn: () => api.syncClaim(w.id),
    onSuccess: () => {
      toast.success("You're next — checkout happens automatically at the next check-in");
      invalidate();
    },
    onError: onError("Claim refused"),
  });
  const unclaim = useMutation({
    mutationFn: () => api.syncUnclaim(w.id),
    onSuccess: invalidate,
    onError: onError("Could not withdraw the claim"),
  });
  const renew = useMutation({
    mutationFn: () => api.syncRenew(status.holder!.sessionId),
    onSuccess: (out) => {
      toast.success(`Hold extended until ${new Date(out.expiresAt).toLocaleString()}`);
      invalidate();
    },
    onError: onError("Renew failed"),
  });
  const release = useMutation({
    mutationFn: () => api.syncRelease(w.id),
    onSuccess: () => {
      toast.success("Hold released");
      invalidate();
    },
    onError: onError("Release failed"),
  });
  const importWorld = useMutation({
    mutationFn: (file: File) => api.syncImport(w.id, file),
    onSuccess: () => {
      toast.success("Imported as the new head");
      invalidate();
    },
    onError: onError("Import refused"),
  });
  const removeWorld = useMutation({
    mutationFn: () => api.deleteSyncWorld(w.id),
    onSuccess: () => {
      toast.success("World deleted");
      queryClient.invalidateQueries({ queryKey: ["syncWorlds"] });
    },
    onError: onError("Delete failed"),
  });

  const mine = status.holder?.username === username;
  const claimedByMe = !!username && status.claimedBy === username;

  return (
    <WkPanel
      title={w.name}
      meta={status.head ? `head v${status.head.id} · ${sizeLabel(status.head.bytes)} · ${agoLabel(status.head.createdAt)}` : "no versions yet"}
    >
      <p className="text-sm text-wk-parchment">{holderLine(status, username)}</p>
      {status.holder && status.claimedBy && (
        <p className="mt-0.5 text-xs text-wk-mist">Next claim: {status.claimedBy}</p>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {canSync && !status.holder && (
          <button className={primaryBtn} disabled={checkout.isPending} onClick={() => checkout.mutate(false)}>
            Check out
          </button>
        )}
        {canSync && status.holder && !mine && status.holder.claimable && (
          <button
            className={primaryBtn}
            disabled={checkout.isPending}
            onClick={() => {
              if (
                confirm(
                  `${status.holder!.username}'s hold has expired. Take the world over? Their late check-in will be kept and flagged, not lost.`,
                )
              )
                checkout.mutate(true);
            }}
          >
            Take over expired hold
          </button>
        )}
        {canSync && mine && (
          <>
            <UploadButton
              label={checkin.isPending ? "Checking in…" : "Check in…"}
              className={primaryBtn}
              disabled={checkin.isPending}
              onFile={(f) => checkin.mutate(f)}
            />
            <button className={quietBtn} disabled={renew.isPending} onClick={() => renew.mutate()}>
              Renew hold
            </button>
          </>
        )}
        {canSync && status.holder && !mine && !claimedByMe && !status.claimedBy && (
          <button className={quietBtn} disabled={claim.isPending} onClick={() => claim.mutate()}>
            Claim next
          </button>
        )}
        {canSync && claimedByMe && (
          <button className={quietBtn} disabled={unclaim.isPending} onClick={() => unclaim.mutate()}>
            Withdraw claim
          </button>
        )}
        {status.head && (
          <a href={api.syncDownloadURL(w.id, status.head.id)} className={quietBtn}>
            Download head
          </a>
        )}
        <button className={quietBtn} onClick={() => setShowHistory(!showHistory)}>
          {showHistory ? "Hide history" : "History"}
        </button>
        {isAdmin && (
          <>
            {!status.holder && (
              <UploadButton
                label={importWorld.isPending ? "Importing…" : "Import…"}
                className={quietBtn}
                disabled={importWorld.isPending}
                onFile={(f) => importWorld.mutate(f)}
              />
            )}
            {status.holder && (
              <button
                className={dangerBtn}
                disabled={release.isPending}
                onClick={() => {
                  if (confirm(`Force-release ${status.holder!.username}'s hold on ${w.name}?`)) release.mutate();
                }}
              >
                Force release
              </button>
            )}
            <button className={quietBtn} onClick={() => setShowSettings(!showSettings)}>
              Settings
            </button>
            <button
              className={dangerBtn}
              onClick={() => {
                if (confirm(`Delete ${w.name} and every stored version? This cannot be undone.`)) removeWorld.mutate();
              }}
            >
              Delete
            </button>
          </>
        )}
      </div>

      {showSettings && isAdmin && <WorldSettings status={status} onDone={() => setShowSettings(false)} />}
      {showHistory && <WorldHistory world={w.id} headVersion={w.headVersion} />}
    </WkPanel>
  );
}

/** The personal companion credential: minted here, pasted into the app. */
function CompanionTokenPanel() {
  const queryClient = useQueryClient();
  const tokenQuery = useQuery({ queryKey: ["syncToken"], queryFn: api.getSyncToken });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["syncToken"] });
  const mint = useMutation({
    mutationFn: api.mintSyncToken,
    onSuccess: () => {
      toast.success("Token minted — paste it into your companion app");
      invalidate();
    },
    onError: (e) => toast.error("Could not mint a token", { description: errorDetail(e) }),
  });
  const revoke = useMutation({
    mutationFn: api.revokeSyncToken,
    onSuccess: () => {
      toast.success("Token revoked");
      invalidate();
    },
    onError: (e) => toast.error("Could not revoke the token", { description: errorDetail(e) }),
  });

  const token = tokenQuery.data?.token ?? "";
  return (
    <WkPanel title="Your companion token">
      <p className="text-sm text-wk-mist">
        The Artificer Companion app on your machine uses this token to check worlds out and in for you. It is yours
        alone — minting a new one revokes the old everywhere.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {token ? (
          <>
            <code className="rounded-sm bg-wk-ink px-2.5 py-1 font-mono text-xs text-wk-parchment">{token}</code>
            <button
              className={quietBtn}
              onClick={() => {
                navigator.clipboard?.writeText(token);
                toast.success("Copied");
              }}
            >
              Copy
            </button>
            <button className={dangerBtn} disabled={revoke.isPending} onClick={() => revoke.mutate()}>
              Revoke
            </button>
          </>
        ) : (
          <button className={primaryBtn} disabled={mint.isPending} onClick={() => mint.mutate()}>
            Mint a token
          </button>
        )}
      </div>
    </WkPanel>
  );
}

export function Worlds() {
  const { isAdmin, can } = useAuth();
  const queryClient = useQueryClient();
  const [newName, setNewName] = useState("");
  // 15s keeps holder state honest across the group without hammering.
  const worldsQuery = useQuery({
    queryKey: ["syncWorlds"],
    queryFn: api.listSyncWorlds,
    refetchInterval: 15_000,
  });
  const create = useMutation({
    mutationFn: () => api.createSyncWorld(newName.trim()),
    onSuccess: () => {
      toast.success("World created");
      setNewName("");
      queryClient.invalidateQueries({ queryKey: ["syncWorlds"] });
    },
    onError: (e) => toast.error("Could not create the world", { description: errorDetail(e) }),
  });

  const worlds = worldsQuery.data?.worlds ?? [];
  const syncAbsent = worldsQuery.isError && (worldsQuery.error as Error & { status?: number }).status === 404;

  return (
    <div className="wildskeeper min-h-full font-wkbody">
      <div className="mx-auto max-w-[1180px] space-y-3.5 p-4 lg:p-7">
        {syncAbsent && (
          <WkPanel title="Shared worlds">
            <p className="text-sm text-wk-mist">Save sync is not enabled on this console.</p>
          </WkPanel>
        )}
        {!syncAbsent && (
          <>
            {worldsQuery.isLoading && (
              <WkPanel title="Shared worlds">
                <p className="text-sm text-wk-mist">Reading the ledger…</p>
              </WkPanel>
            )}
            {worlds.map((status) => (
              <WorldPanel key={status.world.id} status={status} />
            ))}
            {worldsQuery.data && worlds.length === 0 && (
              <WkPanel title="Shared worlds">
                <p className="text-sm text-wk-mist">
                  No shared worlds yet.{" "}
                  {isAdmin ? "Create one below and import its save." : "An admin creates them."}
                </p>
              </WkPanel>
            )}
            {isAdmin && (
              <WkPanel title="New shared world">
                <div className="flex flex-wrap items-center gap-2">
                  <input
                    className="rounded-sm border border-wk-edge bg-wk-ink px-2 py-1.5 text-sm text-wk-parchment"
                    placeholder="World name"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                  />
                  <button
                    className={primaryBtn}
                    disabled={create.isPending || newName.trim() === ""}
                    onClick={() => create.mutate()}
                  >
                    Create
                  </button>
                </div>
                <WkNote>
                  One world, one holder at a time: whoever checks it out hosts, everyone else joins them, and the
                  check-in becomes the canonical version the next host starts from.
                </WkNote>
              </WkPanel>
            )}
            {can("savesync") && <CompanionTokenPanel />}
          </>
        )}
      </div>
    </div>
  );
}
