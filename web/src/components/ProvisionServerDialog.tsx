import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Download, Rocket } from "lucide-react";
import { toast } from "sonner";
import { api, type ProvisionInput, type ProvisionResult } from "../lib/api";
import { copyText } from "../lib/utils";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { NumberField } from "./ui/number-field";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./ui/dialog";

const emptyForm: ProvisionInput = {
  name: "",
  host: "",
  dataPath: "",
  gamePort: 8211,
  restPort: 8212,
  rconPort: 25575,
  agentPort: 8811,
  imageTag: "latest",
  adminPassword: "",
  serverName: "",
  serverDesc: "",
  runAs: "568:568",
};

/**
 * The new-server wizard: palcon registers a fully wired server row and
 * generates the supervisor-mode stack file — the one manual step left is
 * pasting it into a new stack and deploying. The agent installs the game
 * on first boot and comes up already connected to the dashboard.
 */
export function ProvisionServerDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [form, setForm] = useState<ProvisionInput>(emptyForm);
  const [result, setResult] = useState<ProvisionResult | null>(null);
  const [copied, setCopied] = useState(false);

  // The provisioner's configuration answers most of the form; the wizard
  // only asks what it genuinely can't know.
  const defaultsQuery = useQuery({
    queryKey: ["provision-defaults"],
    queryFn: () => api.provisionDefaults(),
    enabled: open,
    staleTime: 60_000,
  });
  const defaults = defaultsQuery.data;
  const oneClick = defaults?.available === true;

  useEffect(() => {
    if (open) {
      setForm(emptyForm);
      setResult(null);
      setCopied(false);
    }
  }, [open]);

  // Prefill once the defaults arrive (the form was just reset on open, so
  // nothing user-typed gets clobbered).
  useEffect(() => {
    if (open && defaults?.available) {
      setForm((f) => ({
        ...f,
        host: defaults.host || f.host,
        imageTag: defaults.imageTag || f.imageTag,
        runAs: defaults.runAs || f.runAs,
        gamePort: defaults.ports?.game ?? f.gamePort,
        restPort: defaults.ports?.rest ?? f.restPort,
        rconPort: defaults.ports?.rcon ?? f.rconPort,
        agentPort: defaults.ports?.agent ?? f.agentPort,
      }));
    }
  }, [open, defaults]);

  const slug = (form.name || "server").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");

  const provision = useMutation({
    mutationFn: (input: ProvisionInput) => api.provisionServer(input),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      setResult(res);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to provision server"),
  });

  const copyStack = async () => {
    if (await copyText(result?.stack ?? "")) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } else {
      toast.error("Copy failed — select the text and copy manually");
    }
  };

  const downloadStack = () => {
    if (!result) return;
    const blob = new Blob([result.stack], { type: "text/yaml" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${result.server.name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}-stack.yml`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] w-[calc(100vw-2rem)] max-w-2xl flex-col overflow-y-auto">
        {!result ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              provision.mutate(form);
            }}
          >
            <DialogHeader>
              <DialogTitle>Provision a new server</DialogTitle>
              <DialogDescription>
                Palcon registers the server and generates a ready-to-deploy stack. The agent installs
                Palworld on first boot — no game image, no manual config.
              </DialogDescription>
            </DialogHeader>

            <div className="mt-4 grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>Name</Label>
                <Input value={form.name} placeholder="Palhalla II" onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label>Host</Label>
                <Input value={form.host} placeholder="10.0.0.5" onChange={(e) => setForm({ ...form, host: e.target.value })} />
                <p className="text-xs text-muted-foreground">Address palcon reaches the box on.</p>
              </div>
            </div>

            {oneClick ? (
              <p className="mt-4 rounded-xl border border-ink/10 bg-ink/5 px-3 py-2 font-mono text-xs text-ink/50">
                data: {defaults?.dataRoot}/{slug || "<name>"} · managed by the provisioner
              </p>
            ) : (
              <div className="mt-4 space-y-1.5">
                <Label>Data path</Label>
                <Input
                  value={form.dataPath}
                  placeholder="/mnt/pool/apps/palworld-palhalla2"
                  onChange={(e) => setForm({ ...form, dataPath: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">
                  Host directory for the install volume — the game (~10&nbsp;GB) and its saves live here.
                </p>
              </div>
            )}

            <div className="mt-4 grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>In-game name</Label>
                <Input
                  value={form.serverName ?? ""}
                  placeholder={form.name || "same as display name"}
                  onChange={(e) => setForm({ ...form, serverName: e.target.value })}
                />
              </div>
              <div className="space-y-1.5">
                <Label>Description / MOTD</Label>
                <Input
                  value={form.serverDesc ?? ""}
                  placeholder="shown in the server browser"
                  onChange={(e) => setForm({ ...form, serverDesc: e.target.value })}
                />
              </div>
            </div>

            <MaybeAdvanced advanced={oneClick}>
            <div className="mt-4 grid grid-cols-4 gap-3">
              {(
                [
                  ["Game (UDP)", "gamePort"],
                  ["REST", "restPort"],
                  ["RCON", "rconPort"],
                  ["Agent", "agentPort"],
                ] as const
              ).map(([label, key]) => (
                <div key={key} className="space-y-1.5">
                  <Label>{label}</Label>
                  <NumberField value={form[key]} onChange={(v) => setForm({ ...form, [key]: v })} min={1} />
                </div>
              ))}
            </div>

            <div className="mt-4 grid grid-cols-3 gap-3">
              <div className="space-y-1.5">
                <Label>Image tag</Label>
                <Input value={form.imageTag} onChange={(e) => setForm({ ...form, imageTag: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label>Run as</Label>
                <Input
                  value={form.runAs ?? ""}
                  placeholder="568:568"
                  onChange={(e) => setForm({ ...form, runAs: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">uid:gid, or "root".</p>
              </div>
              <div className="space-y-1.5">
                <Label>Admin password</Label>
                <Input
                  type="password"
                  value={form.adminPassword ?? ""}
                  placeholder="generated if blank"
                  onChange={(e) => setForm({ ...form, adminPassword: e.target.value })}
                />
              </div>
            </div>
            </MaybeAdvanced>

            <DialogFooter className="mt-6">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" className="clip-notch" disabled={provision.isPending}>
                {provision.isPending
                  ? oneClick
                    ? "Deploying..."
                    : "Generating..."
                  : oneClick
                    ? "Deploy server"
                    : "Generate server"}
              </Button>
            </DialogFooter>
          </form>
        ) : result.deployed ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Rocket className="h-5 w-5 text-pal-green" />
                "{result.server.name}" is deploying
              </DialogTitle>
              <DialogDescription>
                The provisioner created and started the stack — nothing to paste. The agent is
                installing Palworld right now; first install takes a few minutes.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-1 rounded-xl border border-ink/10 bg-ink/5 p-3 font-mono text-xs text-ink/60">
              <p>container&nbsp;&nbsp;palagent-{result.server.name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}</p>
              {result.dataDir && <p>data&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{result.dataDir}</p>}
              <p>admin pw&nbsp;&nbsp;{result.adminPassword}</p>
            </div>
            <p className="text-xs text-ink/50">
              Install progress shows on the server's card (Update log has the SteamCMD output).
              REST/RCON connect on their own once the game is up.
            </p>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Done
              </Button>
              <Button
                className="clip-notch"
                onClick={() => {
                  onOpenChange(false);
                  navigate(`/servers/${result.server.id}`);
                }}
              >
                Go to server
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Deploy "{result.server.name}"</DialogTitle>
              <DialogDescription>
                The server is registered and waiting. One step left: run this stack where the server
                should live.
              </DialogDescription>
            </DialogHeader>

            {result.deployError && (
              <p className="rounded-lg bg-brand-red/10 px-3 py-2 text-xs text-brand-red">
                One-click deploy failed ({result.deployError}) — deploy manually below.
              </p>
            )}

            <div className="flex items-center gap-2">
              <button
                className="flex items-center gap-1.5 rounded-lg border border-ink/15 px-2.5 py-1.5 text-xs font-semibold text-ink/60 hover:bg-ink/5"
                onClick={copyStack}
              >
                {copied ? <Check className="h-3.5 w-3.5 text-pal-green" /> : <Copy className="h-3.5 w-3.5" />}
                {copied ? "Copied" : "Copy"}
              </button>
              <button
                className="flex items-center gap-1.5 rounded-lg border border-ink/15 px-2.5 py-1.5 text-xs font-semibold text-ink/60 hover:bg-ink/5"
                onClick={downloadStack}
              >
                <Download className="h-3.5 w-3.5" />
                Download
              </button>
              <span className="text-xs text-ink/40">Credentials are baked in — treat it like a password.</span>
            </div>

            <pre className="max-h-72 overflow-auto whitespace-pre rounded-xl bg-ink p-4 font-mono text-[11px] leading-relaxed text-paper/80">
              {result.stack}
            </pre>

            <ol className="list-decimal space-y-1 pl-5 text-xs text-ink/60">
              <li>
                Create the data directory (<code className="font-mono">{form.dataPath}</code>)
                {form.runAs && form.runAs !== "root" && (
                  <>
                    , owned by <code className="font-mono">{form.runAs}</code> — the stack runs as that
                    user
                  </>
                )}
                .
              </li>
              <li>Paste as a new TrueNAS custom app (or compose stack) and deploy.</li>
              <li>
                Come back to the server's dashboard — the install runs on first boot and its progress
                shows on the card. REST/RCON connect on their own once the game is up.
              </li>
            </ol>

            <DialogFooter>
              <Button className="clip-notch" onClick={() => onOpenChange(false)}>
                Done
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

/** Children inline in paste mode; behind a disclosure when the provisioner
 * already answered them. */
function MaybeAdvanced({ advanced, children }: { advanced: boolean; children: React.ReactNode }) {
  if (!advanced) return <>{children}</>;
  return (
    <details className="mt-4 rounded-xl border border-ink/10 px-3 pb-1">
      <summary className="cursor-pointer py-2 text-xs font-semibold text-ink/50">
        Advanced — ports, image, user, password (prefilled from the provisioner)
      </summary>
      {children}
    </details>
  );
}
