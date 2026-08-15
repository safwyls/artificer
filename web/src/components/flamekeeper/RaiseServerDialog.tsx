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
 * Stands up a new Enshrouded server: registers the row, and either
 * deploys the container through Ilmari or hands back the stack to deploy
 * by hand.
 *
 * The join password gets the top of the dialog because it is the one
 * choice with a consequence people miss: Enshrouded's own default config
 * is an *open* server, so "leave it blank" must be a decision made while
 * looking at it, not an accident of skipping a collapsed section.
 * Everything else has a sensible default and stays quiet.
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
  const [joinPassword, setJoinPassword] = useState("");
  const [host, setHost] = useState("");
  const [gamePort, setGamePort] = useState(15637);
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
      setGamePort((p) => (p === 15637 ? defaults.ports!.game : p));
      setAgentPort((p) => (p === 8811 ? defaults.ports!.agent : p));
    }
  }, [defaults]);

  const reset = () => {
    setName("");
    setJoinPassword("");
    setDataPath("");
    setAdvanced(false);
    setResult(null);
  };

  const raise = useMutation({
    mutationFn: () =>
      api.provisionServer({
        name: name.trim(),
        host: host.trim(),
        joinPassword: joinPassword.trim() || undefined,
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

  const ready = name.trim() !== "" && host.trim() !== "" && (hasProvisioner || dataPath.trim() !== "");

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="flamekeeper max-h-[90vh] overflow-y-auto border-fk-edge bg-fk-panel font-fkbody text-fk-bone sm:max-w-xl">
        {result ? (
          <RaiseResult result={result} onClose={() => onOpenChange(false)} />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="font-fkdisplay tracking-[0.06em] text-fk-stonehi">Raise a server</DialogTitle>
              <DialogDescription className="text-fk-lichen">
                {hasProvisioner
                  ? "Ilmari will place the container and install the game. First boot takes a while — the Windows depot plus a Wine prefix — watch it from the server's card."
                  : "No provisioner is configured, so this generates a stack for you to deploy. The server is registered either way."}
              </DialogDescription>
            </DialogHeader>

            {/* The decision that deserves the room: open or private. */}
            <div className="fk-toplight rounded-md border border-fk-edge bg-gradient-to-br from-fk-fog to-[#141b16] px-5 py-4">
              <Label htmlFor="raise-join-password" className="text-sm font-bold tracking-[0.06em] text-fk-stonehi">
                Join password
              </Label>
              <Input
                id="raise-join-password"
                value={joinPassword}
                onChange={(e) => setJoinPassword(e.target.value)}
                placeholder="what friends type at the join screen"
                spellCheck={false}
                className="mt-2 border-fk-edge bg-fk-void font-mono text-sm text-fk-bone"
              />
              <p className="mt-2 text-xs text-fk-lichen">
                Leave blank for an <span className="text-fk-spore">open server</span> — anyone who finds it in the
                server browser can join. The admin password (kick/ban rights at the join screen) is generated for you
                and shown once after the raise.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <LabelledField label="Server name" value={name} onChange={setName} placeholder="Emberhold" />
              <LabelledField
                label="Host address"
                value={host}
                onChange={setHost}
                placeholder="10.0.0.9"
                hint="Where Flamekeeper and players reach it"
              />
              <div className="space-y-1.5">
                <Label className="text-fk-lichen">Game port</Label>
                <NumberField value={gamePort} onChange={setGamePort} min={1} max={65535} />
                <p className="text-xs text-fk-lichen">
                  One UDP port — <span className="font-mono text-fk-bone/70">{gamePort}</span> carries the game and the
                  Steam query both.
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={() => setAdvanced((a) => !a)}
              className="flex items-center gap-1.5 text-xs uppercase tracking-[0.1em] text-fk-lichen transition hover:text-fk-bone"
            >
              <ChevronDown className={cn("h-3.5 w-3.5 transition", advanced && "rotate-180")} />
              Deployment details
            </button>
            {advanced && (
              <div className="grid grid-cols-2 gap-3 rounded-md border border-fk-edge bg-fk-void/40 p-3">
                <div className="space-y-1.5">
                  <Label className="text-fk-lichen">Agent port</Label>
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
                  placeholder={hasProvisioner ? `${defaults?.dataRoot ?? ""}/<name>` : "/mnt/pool/apps/enshrouded"}
                  hint={hasProvisioner ? "Blank = the provisioner decides" : "Required without a provisioner"}
                />
              </div>
            )}

            <div className="mt-1 flex items-center justify-end gap-2">
              <button
                onClick={() => onOpenChange(false)}
                className="rounded border border-fk-edge px-3 py-1.5 text-sm text-fk-lichen transition hover:text-fk-bone"
              >
                Cancel
              </button>
              <button
                onClick={() => raise.mutate()}
                disabled={!ready || raise.isPending}
                className="rounded border border-fk-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-fk-stonehi transition hover:brightness-125 disabled:opacity-40"
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
        <DialogTitle className="font-fkdisplay tracking-[0.06em] text-fk-stonehi">
          {result.deployed ? `"${result.server.name}" is rising` : `"${result.server.name}" is registered`}
        </DialogTitle>
        <DialogDescription className="text-fk-lichen">
          {result.deployed
            ? "The container is up and SteamCMD is installing the game. The server card shows progress; the first start takes a few minutes."
            : "Deploy the stack below on the host, then the server card will come alive."}
        </DialogDescription>
      </DialogHeader>

      {result.deployError && (
        <p className="rounded border border-fk-sporedim bg-fk-spore/5 px-3 py-2 text-sm text-fk-spore">
          The provisioner could not deploy it: {result.deployError}. The stack below still works by hand.
        </p>
      )}

      <Secret label="Admin password" value={result.adminPassword} />
      <p className="-mt-1 text-xs text-fk-lichen">
        Joining with this password grants the admin role — kick and ban from the in-game player menu. Shown once — it
        can be rotated later from Configuration.
      </p>

      {!result.deployed && <CopyBlock label="Deployment stack" value={result.stack} />}
      {result.dataDir && (
        <p className="text-xs text-fk-lichen">
          World data lives at <span className="font-mono text-fk-bone/70">{result.dataDir}</span> and survives the
          container.
        </p>
      )}

      <div className="flex justify-end">
        <button
          onClick={onClose}
          className="rounded border border-fk-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-fk-stonehi transition hover:brightness-125"
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
      <Label className="text-fk-lichen">{label}</Label>
      <div className="mt-1.5 flex items-center gap-2 rounded bg-fk-void px-3 py-2.5">
        <code className="flex-1 break-all font-mono text-sm text-fk-bone">{value}</code>
        <button
          onClick={async () => {
            if (await copyText(value)) {
              setCopied(true);
              setTimeout(() => setCopied(false), 2000);
            }
          }}
          title={`Copy ${label.toLowerCase()}`}
          className="shrink-0 text-fk-lichen transition hover:text-fk-bone"
        >
          {copied ? <Check className="h-4 w-4 text-fk-ok" /> : <Copy className="h-4 w-4" />}
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
        <Label className="text-fk-lichen">{label}</Label>
        <button
          onClick={async () => {
            if (await copyText(value)) {
              setCopied(true);
              setTimeout(() => setCopied(false), 2000);
            }
          }}
          className="flex items-center gap-1.5 text-xs text-fk-stonehi transition hover:text-fk-bone"
        >
          {copied ? <Check className="h-3.5 w-3.5 text-fk-ok" /> : <Copy className="h-3.5 w-3.5" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="mt-1.5 max-h-64 overflow-auto rounded bg-fk-void px-3 py-2.5 font-mono text-xs leading-relaxed text-fk-bone/80">
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
      <Label className="text-fk-lichen">{label}</Label>
      <Input
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className="border-fk-edge bg-fk-void text-fk-bone"
      />
      {hint && <p className="text-xs text-fk-lichen">{hint}</p>}
    </div>
  );
}
