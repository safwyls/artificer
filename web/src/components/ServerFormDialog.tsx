import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type DiscoveredServer, type Server, type ServerWriteInput } from "../lib/api";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { NumberField } from "./ui/number-field";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./ui/dialog";

const emptyForm: ServerWriteInput = {
  name: "",
  host: "",
  rconPort: 25575,
  rconPassword: "",
  restPort: 8212,
  restPassword: "",
  gamePort: 8211,
  useRest: true,
  enabled: true,
  savePath: "",
  configPath: "",
  installPath: "",
  agentUrl: "",
  agentToken: "",
  containerName: "",
};

function formStateFor(mode: "create" | "edit", server?: Server): ServerWriteInput {
  if (mode === "edit" && server) {
    return {
      name: server.name,
      host: server.host,
      rconPort: server.rconPort,
      rconPassword: "",
      restPort: server.restPort,
      restPassword: "",
      gamePort: server.gamePort,
      useRest: server.useRest,
      enabled: server.enabled,
      savePath: server.savePath,
      configPath: server.configPath,
      installPath: server.installPath,
      agentUrl: server.agentUrl,
      agentToken: "",
      containerName: server.containerName,
    };
  }
  return emptyForm;
}

export function ServerFormDialog({
  open,
  onOpenChange,
  mode,
  server,
  onProvision,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "create" | "edit";
  server?: Server;
  /** Create mode only: switch to the new-server wizard instead. */
  onProvision?: () => void;
}) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ServerWriteInput>(() => formStateFor(mode, server));
  // How the game runs decides which fields matter; the toggle keeps the
  // create form from asking companion questions about supervised servers.
  const [kind, setKind] = useState<"companion" | "supervised">("companion");

  // Existing installs on the provisioner's host, offered for adoption.
  const discoverQuery = useQuery({
    queryKey: ["provision-discover"],
    queryFn: () => api.provisionDiscover(),
    enabled: open && mode === "create",
    staleTime: 30_000,
  });
  const defaultsQuery = useQuery({
    queryKey: ["provision-defaults"],
    queryFn: () => api.provisionDefaults(),
    enabled: open && mode === "create",
    staleTime: 60_000,
  });
  const candidates = (discoverQuery.data?.servers ?? []).filter((c) => c.mode === "supervisor");

  const adopt = (c: DiscoveredServer) => {
    const host = defaultsQuery.data?.host || form.host;
    setForm({
      ...form,
      name: form.name || c.name.replace(/^palagent-/, "").replace(/-/g, " "),
      host,
      gamePort: c.gamePort || form.gamePort,
      restPort: c.restPort || form.restPort,
      rconPort: c.rconPort || form.rconPort,
      agentUrl: c.agentPort && host ? `http://${host}:${c.agentPort}` : form.agentUrl,
      useRest: true,
    });
  };

  // Reset to fresh values every time the dialog opens, so stale form state
  // from a previous open (or a different server, in edit mode) doesn't leak in.
  useEffect(() => {
    if (open) {
      setForm(formStateFor(mode, server));
      setKind("companion");
    }
  }, [open, mode, server]);

  const save = useMutation({
    mutationFn: (input: ServerWriteInput) =>
      mode === "create" ? api.createServer(input) : api.updateServer(server!.id, input),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      if (mode === "edit") queryClient.invalidateQueries({ queryKey: ["server", result.id] });
      toast.success(mode === "create" ? `Added "${result.name}"` : `Updated "${result.name}"`);
      onOpenChange(false);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to save server"),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onClick={(e) => e.stopPropagation()}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate(form);
          }}
        >
          <DialogHeader>
            <DialogTitle>{mode === "create" ? "Add a Palworld server" : `Edit "${server?.name}"`}</DialogTitle>
            <DialogDescription>
              Credentials come from your server's <code>PalWorldSettings.ini</code>.
              {mode === "edit" && " Leave a password blank to keep the current one."}
            </DialogDescription>
          </DialogHeader>

          {mode === "create" && onProvision && (
            <button
              type="button"
              onClick={onProvision}
              className="mt-3 w-full rounded-xl border border-dashed border-ink/20 px-3 py-2 text-left text-xs text-ink/60 transition hover:border-ink/40 hover:bg-ink/5"
            >
              Starting from scratch? <span className="font-semibold text-ink">Provision a new server</span> —
              palcon generates the whole deployment for you.
            </button>
          )}

          {mode === "create" && (
            <div className="mt-4">
              <div className="grid grid-cols-2 gap-1 rounded-xl border border-ink/10 bg-ink/5 p-1">
                {(
                  [
                    ["companion", "Companion"],
                    ["supervised", "Supervised"],
                  ] as const
                ).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setKind(value)}
                    className={cn(
                      "rounded-lg px-3 py-1.5 text-xs font-semibold transition",
                      kind === value ? "bg-white text-ink shadow-sm" : "text-ink/50 hover:text-ink",
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                {kind === "companion"
                  ? "The game runs in its own container (an existing server image); a palagent beside it handles files and updates, and power goes through the docker proxy."
                  : "The palagent container runs the game itself — power, updates, files and logs all flow through the agent. No container name or path mounts needed."}
              </p>

              {kind === "supervised" && candidates.length > 0 && (
                <div className="mt-3 space-y-1.5">
                  <p className="text-xs font-semibold text-ink/60">Found on the provisioner's host</p>
                  {candidates.map((c) => (
                    <button
                      key={c.name}
                      type="button"
                      disabled={c.registered}
                      onClick={() => adopt(c)}
                      className="flex w-full items-center gap-2 rounded-xl border border-ink/10 px-3 py-2 text-left text-xs transition hover:border-ink/30 hover:bg-ink/5 disabled:opacity-50"
                    >
                      <span
                        className={cn("h-2 w-2 shrink-0 rounded-full", c.running ? "bg-pal-green" : "bg-ink/30")}
                      />
                      <span className="font-mono">{c.name}</span>
                      <span className="text-ink/40">agent :{c.agentPort || "?"}</span>
                      <span className="ml-auto text-ink/40">
                        {c.registered ? "already added" : "click to prefill"}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          <div className="mt-4 grid grid-cols-2 gap-3">
            <Field label="Name" value={form.name} onChange={(v) => setForm({ ...form, name: v })} />
            <Field label="Host" value={form.host} onChange={(v) => setForm({ ...form, host: v })} />
            <div className="space-y-1.5">
              <Label>REST port</Label>
              <NumberField value={form.restPort} onChange={(v) => setForm({ ...form, restPort: v })} min={0} />
            </div>
            <Field
              label="REST password"
              value={form.restPassword ?? ""}
              onChange={(v) => setForm({ ...form, restPassword: v })}
              type="password"
              placeholder={mode === "edit" && server?.hasRestPassword ? "unchanged" : undefined}
            />
            <div className="space-y-1.5">
              <Label>RCON port</Label>
              <NumberField value={form.rconPort} onChange={(v) => setForm({ ...form, rconPort: v })} min={0} />
            </div>
            <Field
              label="RCON password"
              value={form.rconPassword ?? ""}
              onChange={(v) => setForm({ ...form, rconPassword: v })}
              type="password"
              placeholder={mode === "edit" && server?.hasRconPassword ? "unchanged" : undefined}
            />
            <div className="space-y-1.5">
              <Label>Game port (players)</Label>
              <NumberField value={form.gamePort} onChange={(v) => setForm({ ...form, gamePort: v })} min={1} />
              <p className="text-xs text-muted-foreground">Shown as the join address on the dashboard.</p>
            </div>
          </div>

          {(mode === "edit" || kind === "companion") && (
          <>
          <div className="mt-4 space-y-1.5">
            <Label>Container name (optional)</Label>
            <Input
              value={form.containerName}
              placeholder="palworld"
              onChange={(e) => setForm({ ...form, containerName: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Docker container this server runs in. Enables start/stop/restart, and needs
              <code> DOCKER_HOST</code> pointed at a scoped socket proxy.
            </p>
          </div>

          <div className="mt-4 space-y-1.5">
            <Label>Save path (optional)</Label>
            <Input
              value={form.savePath}
              placeholder="/saves/myserver"
              onChange={(e) => setForm({ ...form, savePath: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Container path to the world save folder (holds <code>Level.sav</code>), mounted read-only.
              Enables the Pal party/palbox viewer. Not needed with an agent — saves sync from it
              automatically.
            </p>
          </div>

          <div className="mt-4 space-y-1.5">
            <Label>Config path (optional)</Label>
            <Input
              value={form.configPath}
              placeholder="/config/myserver"
              onChange={(e) => setForm({ ...form, configPath: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Container path to the folder holding <code>PalWorldSettings.ini</code>, mounted
              <strong> read-write</strong>. Enables the settings editor. Keep this separate from the save
              mount so save data stays read-only. Not needed with an agent.
            </p>
          </div>

          <div className="mt-4 space-y-1.5">
            <Label>Install path (optional)</Label>
            <Input
              value={form.installPath}
              placeholder="/palworld"
              onChange={(e) => setForm({ ...form, installPath: e.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              Container path to the Palworld install root (holds <code>steamapps</code>), mounted
              <strong> read-write</strong>. Enables clearing the SteamCMD cache when a game update
              corrupts it. Not needed with an agent.
            </p>
          </div>
          </>
          )}

          <div className="mt-4 grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label>Agent URL (optional)</Label>
              <Input
                value={form.agentUrl}
                placeholder="http://palagent:8811"
                onChange={(e) => setForm({ ...form, agentUrl: e.target.value })}
              />
            </div>
            <Field
              label="Agent token"
              value={form.agentToken ?? ""}
              onChange={(v) => setForm({ ...form, agentToken: v })}
              type="password"
              placeholder={mode === "edit" && server?.hasAgentToken ? "unchanged" : undefined}
            />
            <p className="col-span-2 text-xs text-muted-foreground">
              The <code>palagent</code> sidecar deployed next to this game server. Replaces all three
              path mounts above: SteamCMD repair and updates, the Pal viewer, the settings editor and
              backups all work through it. Token must match the agent's <code>PALAGENT_TOKEN</code>.
            </p>
          </div>

          <div className="mt-4 flex items-center gap-2">
            <Switch
              id="use-rest"
              checked={form.useRest}
              onCheckedChange={(checked) => setForm({ ...form, useRest: checked })}
            />
            <Label htmlFor="use-rest" className="text-foreground">
              Prefer REST API (falls back to RCON)
            </Label>
          </div>

          <DialogFooter className="mt-6">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" className="clip-notch" disabled={save.isPending}>
              {save.isPending ? "Saving..." : mode === "create" ? "Add server" : "Save changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  value,
  onChange,
  type = "text",
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  placeholder?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      <Input type={type} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
