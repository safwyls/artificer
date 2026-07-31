import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Download } from "lucide-react";
import { toast } from "sonner";
import { api, type ProvisionInput, type ProvisionResult } from "../lib/api";
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
  const [form, setForm] = useState<ProvisionInput>(emptyForm);
  const [result, setResult] = useState<ProvisionResult | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (open) {
      setForm(emptyForm);
      setResult(null);
      setCopied(false);
    }
  }, [open]);

  const provision = useMutation({
    mutationFn: (input: ProvisionInput) => api.provisionServer(input),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      setResult(res);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to provision server"),
  });

  const copyStack = async () => {
    await navigator.clipboard.writeText(result?.stack ?? "");
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
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

            <div className="mt-4 grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>Image tag</Label>
                <Input value={form.imageTag} onChange={(e) => setForm({ ...form, imageTag: e.target.value })} />
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

            <DialogFooter className="mt-6">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" className="clip-notch" disabled={provision.isPending}>
                {provision.isPending ? "Generating..." : "Generate server"}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Deploy "{result.server.name}"</DialogTitle>
              <DialogDescription>
                The server is registered and waiting. One step left: run this stack where the server
                should live.
              </DialogDescription>
            </DialogHeader>

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
                Create the data directory (<code className="font-mono">{form.dataPath}</code>), owned
                by <code className="font-mono">568:568</code> (apps) — the stack runs as that user.
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
