import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import { api, errorDetail, type WorldWriteInput } from "../lib/api";
import { useArtwork } from "../lib/art";
import { useAuth } from "../lib/auth";
import { fmtBytes } from "../lib/format";
import { POLL_MS } from "../lib/live";
import { useWorldMutations } from "../lib/mutations";
import { at, worldActions } from "../lib/worldActions";
import { CoverArt } from "../components/CoverArt";
import { CustodyChip, holderLine, requestLine } from "../components/CustodyChip";
import { VersionRow } from "../components/VersionRow";
import { ActionButton, OverflowMenu } from "../components/WorldActions";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { cn } from "../lib/utils";
import type { World, WorldDetail as Detail } from "../lib/types";

const TABS = [
  { id: "history", label: "History" },
  { id: "settings", label: "Settings", admin: true },
  { id: "server", label: "Server link", admin: true },
];

/**
 * The settings the API replaces as one record. Both the Settings tab and
 * the Server link tab write through here with the *whole* record, because a
 * partial PUT clears what it omits — the same trap the user table has, at
 * a different endpoint.
 */
function recordOf(w: World): WorldWriteInput {
  return {
    name: w.name,
    leaseHours: w.leaseHours,
    maxBytes: w.maxBytes,
    keepVersions: w.keepVersions,
    checkpoints: w.checkpoints,
    savePath: w.savePath,
    webhookUrl: w.webhookUrl,
    agentUrl: w.agentUrl,
    // Never returned by the API; empty keeps whatever is stored.
    agentToken: "",
  };
}

function useSettingsDraft(world: World | undefined) {
  const [draft, setDraft] = useState<WorldWriteInput | null>(null);
  // Re-seed when the server's copy changes identity, not on every poll —
  // otherwise a custody event mid-edit would wipe the form.
  useEffect(() => {
    if (world) setDraft(recordOf(world));
  }, [world?.id]); // eslint-disable-line react-hooks/exhaustive-deps
  return [draft, setDraft] as const;
}

function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: React.ReactNode;
  hint?: string;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[11px] uppercase tracking-[0.1em] text-mist">{label}</span>
      {children}
      {hint ? <span className="text-[12px] italic text-mist">{hint}</span> : null}
    </label>
  );
}

export function WorldDetail() {
  const { worldID } = useParams();
  const id = Number(worldID);
  const { username, isAdmin, canSync } = useAuth();
  const [params, setParams] = useSearchParams();
  const queryClient = useQueryClient();

  const detail = useQuery<Detail>({
    queryKey: ["world", id],
    queryFn: () => api.world(id),
    enabled: Number.isFinite(id),
    refetchInterval: POLL_MS,
  });
  const status = detail.data?.status;
  const world = status?.world;
  // One world is still a set of worlds as far as the cover lookup cares.
  useArtwork(useMemo(() => (status ? [status] : []), [status?.world.id])); // eslint-disable-line react-hooks/exhaustive-deps

  const { handlers, upload } = useWorldMutations(id);
  const [draft, setDraft] = useSettingsDraft(world);

  const save = useMutation({
    mutationFn: (input: WorldWriteInput) => api.updateWorld(id, input),
    onSuccess: () => {
      toast.success("settings saved");
      queryClient.invalidateQueries({ queryKey: ["world", id] });
      queryClient.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (err) => toast.error(errorDetail(err)),
  });
  const setHead = useMutation({
    mutationFn: (versionId: number) => api.setHead(id, versionId),
    onSuccess: () => {
      toast.success("head moved");
      queryClient.invalidateQueries({ queryKey: ["world", id] });
      queryClient.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (err) => toast.error(errorDetail(err)),
  });

  if (detail.isLoading) return <p className="p-8 text-mist">Reading the ledger…</p>;
  if (!status || !world) {
    return (
      <div className="p-8">
        <p className="font-mono text-[13px] text-ember">
          {detail.isError ? errorDetail(detail.error) : "no such world"}
        </p>
        <Link to="/" className="mt-3 inline-block text-[13px]">
          Back to the worlds
        </Link>
      </div>
    );
  }

  const tabs = TABS.filter((t) => !t.admin || isAdmin);
  const wanted = params.get("tab") ?? "history";
  const tab = tabs.some((t) => t.id === wanted) ? wanted : "history";
  const actions = worldActions(status, { username, isAdmin, canSync }, handlers);
  const onUpload = (kind: "checkin" | "import", file: File) =>
    upload(kind, file, status.holder?.sessionId);
  const asked = requestLine(status);
  const versions = detail.data?.versions ?? [];
  const stored = versions.reduce((sum, v) => sum + v.bytes, 0);

  return (
    <>
      <div className="border-b border-edge px-4 pb-3.5 pt-4 md:px-8 md:pb-[18px] md:pt-[22px]">
        <div className="mb-3 text-[12px] text-mist">
          <Link to="/" className="no-underline">
            Worlds
          </Link>{" "}
          <span className="text-edge">/</span> {world.name}
        </div>
        <div className="flex items-start gap-5">
          <CoverArt world={world} size="detail" />
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <div className="flex flex-wrap items-baseline gap-3">
              <span className="text-[24px] font-bold">{world.name}</span>
              {world.gameTitle ? <span className="text-[13px] text-rune">{world.gameTitle}</span> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2.5">
              <CustodyChip status={status} />
              <span className="text-[13px] text-mist">{holderLine(status, username)}</span>
            </div>
            {asked ? <div className="font-mono text-[12px] text-rune">{asked}</div> : null}
            <div className="flex flex-wrap gap-x-6 gap-y-1 font-mono text-[12px] text-mist [overflow-wrap:anywhere]">
              <span>
                {status.head ? `head v${status.head.id} · ${fmtBytes(status.head.bytes)}` : "no versions yet"}
              </span>
              {world.saveHint ? (
                <span title="where the companion that linked this game found its save">
                  save location: {world.saveHint}
                </span>
              ) : null}
              {world.savePath ? (
                <span title="created inside each player's own save folder when they link this world">
                  world folder: {world.savePath}
                </span>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {at(actions, "detail", "primary").map((a) => (
                <ActionButton key={a.id} action={a} placement="primary" onUpload={onUpload} />
              ))}
              {at(actions, "detail", "quiet").map((a) => (
                <ActionButton key={a.id} action={a} placement="quiet" onUpload={onUpload} />
              ))}
              <OverflowMenu
                actions={at(actions, "detail", "overflow")}
                onUpload={onUpload}
                label={`More actions for ${world.name}`}
              />
            </div>
          </div>
        </div>
        <div className="mt-3.5 flex gap-1 overflow-x-auto md:mt-[18px]" role="tablist">
          {tabs.map((t) => (
            <button
              key={t.id}
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => setParams(t.id === "history" ? {} : { tab: t.id }, { replace: true })}
              className={cn(
                "px-[18px] py-1.5 text-[13px]",
                tab === t.id ? "border-b-2 border-gold text-goldhi" : "text-mist hover:text-parchment",
              )}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {tab === "history" ? (
        <div className="px-4 py-4 md:px-8 md:py-[22px]">
          {versions.length ? (
            <div className="overflow-hidden rounded-panel border border-edge bg-panel">
              {versions.map((v) => (
                <VersionRow
                  key={v.id}
                  version={v}
                  worldID={id}
                  isHead={world.headVersion === v.id}
                  uploader={
                    v.uploaderId != null
                      ? (detail.data?.uploaders?.[String(v.uploaderId)] ?? `user #${v.uploaderId}`)
                      : "unknown"
                  }
                  canSetHead={isAdmin}
                  onSetHead={(versionID) => setHead.mutate(versionID)}
                />
              ))}
            </div>
          ) : (
            <div className="rounded-panel border border-edge bg-panel px-5 py-4 text-[13px] text-mist">
              No versions yet — the first check-in or import starts the history.
            </div>
          )}
          <div className="mt-2.5 text-[12px] italic text-mist">
            Keeping the last {world.keepVersions} versions · {fmtBytes(stored)} of{" "}
            {fmtBytes(world.maxBytes)} per version · a conflict badge means an admin should pick a head.
          </div>
        </div>
      ) : null}

      {tab === "settings" && isAdmin && draft ? (
        <form
          className="grid max-w-4xl grid-cols-1 gap-4 px-4 py-4 sm:grid-cols-2 md:px-8 md:py-[22px]"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            save.mutate(draft);
          }}
        >
          <Field label="Name">
            <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </Field>
          <Field label="Lease (hours)">
            <Input
              type="number"
              value={draft.leaseHours}
              onChange={(e) => setDraft({ ...draft, leaseHours: Number(e.target.value) })}
            />
          </Field>
          <Field label="Max size (MiB)">
            <Input
              type="number"
              value={Math.round(draft.maxBytes / (1 << 20))}
              onChange={(e) => setDraft({ ...draft, maxBytes: Number(e.target.value) * (1 << 20) })}
            />
          </Field>
          <Field label="Keep versions">
            <Input
              type="number"
              value={draft.keepVersions}
              onChange={(e) => setDraft({ ...draft, keepVersions: Number(e.target.value) })}
            />
          </Field>
          <Field
            label="World folder"
            hint="Inside each player's own save folder; usually blank, e.g. K2hAc0p_LH74aymwOemkgg"
          >
            <Input
              value={draft.savePath}
              onChange={(e) => setDraft({ ...draft, savePath: e.target.value.trim() })}
            />
          </Field>
          <Field label="Discord webhook URL">
            <Input
              value={draft.webhookUrl}
              onChange={(e) => setDraft({ ...draft, webhookUrl: e.target.value })}
            />
          </Field>
          <label className="col-span-2 flex items-center gap-2 text-[14px]">
            <input
              type="checkbox"
              checked={draft.checkpoints}
              onChange={(e) => setDraft({ ...draft, checkpoints: e.target.checked })}
            />
            Mid-session checkpoints
          </label>
          <div className="col-span-2">
            <Button type="submit" variant="primary" disabled={save.isPending}>
              Save settings
            </Button>
          </div>
        </form>
      ) : null}

      {tab === "server" && isAdmin && draft ? (
        <form
          className="max-w-2xl px-4 py-4 md:px-8 md:py-[22px]"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            save.mutate(draft);
          }}
        >
          <p className="mb-4 text-[13px] text-mist">
            A dedicated server can hold this world like a player does: give it the world, and take it back when the
            session ends. Both need the sidecar agent&apos;s address and its token.
          </p>
          <div className="flex flex-col gap-4">
            <Field label="Dedicated-server agent URL">
              <Input
                placeholder="http://host:8420"
                value={draft.agentUrl}
                onChange={(e) => setDraft({ ...draft, agentUrl: e.target.value })}
              />
            </Field>
            <Field
              label={`Agent token${world.hasAgentToken ? " (saved — empty keeps it)" : ""}`}
              hint="Clearing the URL clears the credential with it."
            >
              <Input
                type="password"
                value={draft.agentToken}
                onChange={(e) => setDraft({ ...draft, agentToken: e.target.value })}
              />
            </Field>
            <div className="flex items-center gap-2">
              <Button type="submit" variant="primary" disabled={save.isPending}>
                Save server link
              </Button>
              {at(actions, "detail", "quiet")
                .filter((a) => a.id === "server-give" || a.id === "server-take")
                .map((a) => (
                  <ActionButton key={a.id} action={a} placement="quiet" onUpload={onUpload} />
                ))}
            </div>
          </div>
        </form>
      ) : null}
    </>
  );
}
