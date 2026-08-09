import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, Copy } from "lucide-react";
import { toast } from "sonner";
import { api, type ProvisionResult } from "../../lib/api";
import { cn, copyText } from "../../lib/utils";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../ui/dialog";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { NumberField } from "../ui/number-field";

/**
 * Stands up a new Dragonwilds server: registers the row, and either
 * deploys the container through a provisioner or hands back the stack to
 * deploy by hand.
 *
 * The Owner ID gets the whole top of the dialog because it is the one
 * field with no default and no second chance — the game refuses to start
 * without it, and it lives somewhere non-obvious (in-game Settings), so
 * anyone who hasn't looked it up first is stuck. Everything else has a
 * sensible default and stays quiet.
 */
export function RaiseServerDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [ownerId, setOwnerId] = useState("");
  const [host, setHost] = useState("");
  const [worldName, setWorldName] = useState("");
  const [gamePort, setGamePort] = useState(7777);
  const [agentPort, setAgentPort] = useState(8811);
  const [dataPath, setDataPath] = useState("");
  const [imageTag, setImageTag] = useState("latest");
  const [runAs, setRunAs] = useState("568:568");
  const [advanced, setAdvanced] = useState(false);
  const [result, setResult] = useState<ProvisionResult | null>(null);

  const defaultsQuery = useQuery({
    queryKey: ["provision-defaults"],
    queryFn: api.provisionDefaults,
    enabled: open,
  });
  const defaults = defaultsQuery.data;
  const hasProvisioner = Boolean(defaults?.available);

  // Prefill once the provisioner answers, without clobbering typing.
  useEffect(() => {
    if (!defaults?.available) return;
    setHost((h) => h || defaults.host || "");
    setRunAs((r) => (r === "568:568" && defaults.runAs ? defaults.runAs : r));
    setImageTag((t) => (t === "latest" && defaults.imageTag ? defaults.imageTag : t));
    if (defaults.ports) {
      setGamePort((p) => (p === 7777 ? defaults.ports!.game : p));
      setAgentPort((p) => (p === 8811 ? defaults.ports!.agent : p));
    }
  }, [defaults]);

  const reset = () => {
    setName("");
    setOwnerId("");
    setWorldName("");
    setDataPath("");
    setAdvanced(false);
    setResult(null);
  };

  const raise = useMutation({
    mutationFn: () =>
      api.provisionServer({
        name: name.trim(),
        host: host.trim(),
        ownerId: ownerId.trim(),
        worldName: worldName.trim() || undefined,
        dataPath: dataPath.trim(),
        gamePort,
        agentPort,
        imageTag: imageTag.trim() || "latest",
        runAs: runAs.trim(),
      }),
    onSuccess: (res) => {
      setResult(res);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      if (res.deployed) toast.success(`"${res.server.name}" deployed`);
    },
    onError: (e: Error) => toast.error(e.message || "Could not raise the server"),
  });

  const ready = name.trim() !== "" && ownerId.trim() !== "" && host.trim() !== "" && (hasProvisioner || dataPath.trim() !== "");

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="wildskeeper max-h-[90vh] overflow-y-auto border-wk-edge bg-wk-panel font-wkbody text-wk-parchment sm:max-w-xl">
        {result ? (
          <RaiseResult result={result} onClose={() => onOpenChange(false)} />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="font-wkdisplay tracking-[0.06em] text-wk-brasshi">Raise a server</DialogTitle>
              <DialogDescription className="text-wk-mist">
                {hasProvisioner
                  ? "The provisioner will create the container and install the game. First boot takes a few minutes — watch it from the server's card."
                  : "No provisioner is configured, so this generates a stack for you to deploy. The server is registered either way."}
              </DialogDescription>
            </DialogHeader>

            {/* The prerequisite, given the room it earns. */}
            <div className="wk-corners rounded-md border border-wk-brass bg-gradient-to-br from-wk-raise to-[#131a24] px-5 py-4">
              <Label htmlFor="raise-owner-id" className="font-wkdisplay text-sm tracking-[0.06em] text-wk-brasshi">
                Owner ID
              </Label>
              <Input
                id="raise-owner-id"
                value={ownerId}
                onChange={(e) => setOwnerId(e.target.value)}
                placeholder="0a1b2c3d4e5f60718293a4b5c6d7e8f9"
                spellCheck={false}
                className="mt-2 border-wk-edge bg-wk-ink font-mono text-sm text-wk-parchment"
              />
              <p className="mt-2 text-xs text-wk-mist">
                In game: <span className="text-wk-rune">Settings</span> → bottom-left{" "}
                <span className="text-wk-rune">My Player ID</span> → copy. Works the same on Steam or Epic. The server
                will not start without it, and this ID becomes the Owner — the only role that can unban.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <LabelledField label="Server name" value={name} onChange={setName} placeholder="Grimwood Bastion" />
              <LabelledField
                label="Host address"
                value={host}
                onChange={setHost}
                placeholder="10.0.0.9"
                hint="Where Wildskeeper and players reach it"
              />
              <LabelledField
                label="World name"
                value={worldName}
                onChange={setWorldName}
                placeholder="optional"
                hint="Names the world created on first boot"
              />
              <div className="space-y-1.5">
                <Label className="text-wk-mist">Game port</Label>
                <NumberField value={gamePort} onChange={setGamePort} min={1} max={65534} />
                <p className="text-xs text-wk-mist">
                  Publishes <span className="font-mono text-wk-parchment/70">{gamePort}</span> and{" "}
                  <span className="font-mono text-wk-parchment/70">{gamePort + 1}</span> — the game uses both.
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={() => setAdvanced((a) => !a)}
              className="flex items-center gap-1.5 text-xs uppercase tracking-[0.1em] text-wk-mist transition hover:text-wk-parchment"
            >
              <ChevronDown className={cn("h-3.5 w-3.5 transition", advanced && "rotate-180")} />
              Deployment details
            </button>
            {advanced && (
              <div className="grid grid-cols-2 gap-3 rounded-md border border-wk-edge bg-wk-ink/40 p-3">
                <div className="space-y-1.5">
                  <Label className="text-wk-mist">Agent port</Label>
                  <NumberField value={agentPort} onChange={setAgentPort} min={1} max={65535} />
                </div>
                <LabelledField
                  label="Image tag"
                  value={imageTag}
                  onChange={setImageTag}
                  placeholder="latest"
                />
                <LabelledField
                  label="Run as"
                  value={runAs}
                  onChange={setRunAs}
                  placeholder="568:568"
                  hint='uid:gid, or "root"'
                />
                <LabelledField
                  label="Data path"
                  value={dataPath}
                  onChange={setDataPath}
                  placeholder={hasProvisioner ? `${defaults?.dataRoot ?? ""}/<name>` : "/mnt/pool/apps/dragonwilds"}
                  hint={hasProvisioner ? "Blank = the provisioner decides" : "Required without a provisioner"}
                />
              </div>
            )}

            <div className="mt-1 flex items-center justify-end gap-2">
              <button
                onClick={() => onOpenChange(false)}
                className="rounded border border-wk-edge px-3 py-1.5 text-sm text-wk-mist transition hover:text-wk-parchment"
              >
                Cancel
              </button>
              <button
                onClick={() => raise.mutate()}
                disabled={!ready || raise.isPending}
                className="rounded border border-wk-brass bg-gradient-to-b from-[#2a2416] to-[#1e1a10] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-wk-brasshi transition hover:brightness-125 disabled:opacity-40"
              >
                {raise.isPending ? "Raising…" : hasProvisioner ? "Raise the server" : "Generate the stack"}
              </button>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

/** The one-time reveal: what was made, and the secrets that won't be shown again. */
function RaiseResult({ result, onClose }: { result: ProvisionResult; onClose: () => void }) {
  return (
    <>
      <DialogHeader>
        <DialogTitle className="font-wkdisplay tracking-[0.06em] text-wk-brasshi">
          {result.deployed ? `"${result.server.name}" is rising` : `"${result.server.name}" is registered`}
        </DialogTitle>
        <DialogDescription className="text-wk-mist">
          {result.deployed
            ? "The container is up and SteamCMD is installing the game. The server card shows progress; the first start takes a few minutes."
            : "Deploy the stack below on the host, then the server card will come alive."}
        </DialogDescription>
      </DialogHeader>

      {result.deployError && (
        <p className="rounded border border-wk-emberdim bg-wk-ember/5 px-3 py-2 text-sm text-wk-ember">
          The provisioner could not deploy it: {result.deployError}. The stack below still works by hand.
        </p>
      )}

      <Secret label="Admin password" value={result.adminPassword} />
      <p className="-mt-1 text-xs text-wk-mist">
        This is the password the in-game Server Management menu accepts. Shown once — it can be rotated later from
        Configuration.
      </p>

      {!result.deployed && <CopyBlock label="Deployment stack" value={result.stack} />}
      {result.dataDir && (
        <p className="text-xs text-wk-mist">
          World data lives at <span className="font-mono text-wk-parchment/70">{result.dataDir}</span> and survives the
          container.
        </p>
      )}

      <div className="flex justify-end">
        <button
          onClick={onClose}
          className="rounded border border-wk-brass bg-gradient-to-b from-[#2a2416] to-[#1e1a10] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-wk-brasshi transition hover:brightness-125"
        >
          Done
        </button>
      </div>
    </>
  );
}

function Secret({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div>
      <Label className="text-wk-mist">{label}</Label>
      <div className="mt-1.5 flex items-center gap-2 rounded bg-wk-ink px-3 py-2.5">
        <code className="flex-1 break-all font-mono text-sm text-wk-parchment">{value}</code>
        <button
          onClick={async () => {
            if (await copyText(value)) {
              setCopied(true);
              setTimeout(() => setCopied(false), 2000);
            }
          }}
          title={`Copy ${label.toLowerCase()}`}
          className="shrink-0 text-wk-mist transition hover:text-wk-parchment"
        >
          {copied ? <Check className="h-4 w-4 text-wk-ok" /> : <Copy className="h-4 w-4" />}
        </button>
      </div>
    </div>
  );
}

function CopyBlock({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div>
      <div className="flex items-center justify-between">
        <Label className="text-wk-mist">{label}</Label>
        <button
          onClick={async () => {
            if (await copyText(value)) {
              setCopied(true);
              setTimeout(() => setCopied(false), 2000);
            }
          }}
          className="flex items-center gap-1.5 text-xs text-wk-brasshi transition hover:text-wk-parchment"
        >
          {copied ? <Check className="h-3.5 w-3.5 text-wk-ok" /> : <Copy className="h-3.5 w-3.5" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="mt-1.5 max-h-64 overflow-auto rounded bg-wk-ink px-3 py-2.5 font-mono text-xs leading-relaxed text-wk-parchment/80">
        {value}
      </pre>
    </div>
  );
}

function LabelledField({
  label,
  value,
  onChange,
  placeholder,
  hint,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  hint?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label className="text-wk-mist">{label}</Label>
      <Input
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className="border-wk-edge bg-wk-ink text-wk-parchment"
      />
      {hint && <p className="text-xs text-wk-mist">{hint}</p>}
    </div>
  );
}
