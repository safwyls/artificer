import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type DiscoveredServer } from "../lib/api";
import { cn } from "../lib/utils";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./ui/dialog";
import { ServerFormDialog } from "./ServerFormDialog";
import { ProvisionServerDialog } from "./ProvisionServerDialog";

/**
 * The one front door for the + button. Three ways a server ends up in
 * Palcon — adopt one that's already running, provision a new one, or
 * describe an unmanaged one by hand — and none of them is "first" by
 * decree: the chooser asks the host what's actually there and leads with
 * that. Discovery finding unregistered containers puts adoption on top;
 * a configured provisioner makes "provision" the emphasized path; typing
 * everything by hand is always available and always last, because it is
 * the fallback, not the default.
 */
type Stage = "choose" | "provision" | "manual";

export function AddServerFlow({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient();
  const [stage, setStage] = useState<Stage>("choose");

  // Every open starts at the chooser; the picked path is not sticky.
  useEffect(() => {
    if (open) setStage("choose");
  }, [open]);

  // Closing any stage closes the whole flow — a wizard that bounces you
  // back to the menu after you cancel (or finish) feels like a trap.
  const closeFrom = (next: boolean) => {
    if (!next) onOpenChange(false);
  };

  const discoverQuery = useQuery({
    queryKey: ["provision-discover"],
    queryFn: () => api.provisionDiscover(),
    enabled: open,
    staleTime: 30_000,
  });
  const defaultsQuery = useQuery({
    queryKey: ["provision-defaults"],
    queryFn: () => api.provisionDefaults(),
    enabled: open,
    staleTime: 60_000,
  });
  const hasProvisioner = Boolean(defaultsQuery.data?.available);

  // Which discoveries are offered for adoption. The legacy provisioner
  // reported each container's PALAGENT_MODE, so "supervisor" means a game
  // server and "provisioner" means itself. Ilmari doesn't read container
  // env for discovery, so its candidates arrive with mode "" — unknown is
  // not disqualifying, or the adopt list would be empty for every
  // Ilmari-backed console. The one wrong pick an unknown allows — the
  // legacy provisioner's own container — is refused server-side by adopt,
  // with an explanation. Already-registered containers stay out: they're
  // in the rail already, and this dialog exists to add what isn't.
  const candidates = (discoverQuery.data?.servers ?? []).filter(
    (c) => (c.mode === "supervisor" || c.mode === "") && !c.registered,
  );

  // Adoption is one click end to end: the provisioner recovers the
  // container's own token and passwords, so there is nothing to type.
  const adopt = useMutation({
    mutationFn: (c: DiscoveredServer) => {
      const browserHost = ["localhost", "127.0.0.1"].includes(window.location.hostname)
        ? undefined
        : window.location.hostname;
      return api.adoptServer(c.name, defaultsQuery.data?.host || browserHost);
    },
    onSuccess: ({ server }) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      queryClient.invalidateQueries({ queryKey: ["provision-discover"] });
      toast.success(`Adopted "${server.name}"`);
      onOpenChange(false);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to adopt server"),
  });

  return (
    <>
      <Dialog open={open && stage === "choose"} onOpenChange={closeFrom}>
        {/* Radix's open-autofocus draws the browser's focus outline on the
            first card, which reads as emphasis the layout didn't choose —
            focus stays on the dialog until the keyboard asks for it. */}
        <DialogContent className="w-[calc(100vw-2rem)] max-w-md" onOpenAutoFocus={(e) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>Add a server</DialogTitle>
            <DialogDescription>Where is the server coming from?</DialogDescription>
          </DialogHeader>

          {candidates.length > 0 && (
            <div className="space-y-1.5">
              {/* The host report: this dialog already looked. Recognizing
                  your own container beats understanding our taxonomy. */}
              <p className="text-xs text-muted-foreground">
                {candidates.length === 1
                  ? "One server is running on your host that isn't here yet."
                  : `${candidates.length} servers are running on your host that aren't here yet.`}
              </p>
              {candidates.map((c) => (
                <button
                  key={c.name}
                  type="button"
                  disabled={adopt.isPending}
                  onClick={() => adopt.mutate(c)}
                  className="flex w-full items-center gap-2 rounded-xl border border-ink/10 px-3 py-2 text-left text-xs transition hover:border-brand-amber hover:bg-ink/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-red/40 disabled:opacity-50"
                >
                  <span className={cn("h-2 w-2 shrink-0 rounded-full", c.running ? "bg-pal-green" : "bg-ink/20")} />
                  <span className="font-mono">{c.name}</span>
                  <span className="text-ink/40">agent :{c.agentPort || "?"}</span>
                  <span className="ml-auto font-semibold text-brand-red">
                    {adopt.isPending ? "Adopting…" : "Adopt"}
                  </span>
                </button>
              ))}
            </div>
          )}

          <div className="space-y-2">
            <button
              type="button"
              onClick={() => setStage("provision")}
              className={cn(
                "w-full rounded-xl border px-4 py-3 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-red/40",
                hasProvisioner
                  ? "border-brand-amber bg-brand-amber/5 hover:bg-brand-amber/10"
                  : "border-ink/10 hover:bg-ink/5",
              )}
            >
              <p className="text-sm font-bold text-ink">Provision a new server</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {hasProvisioner
                  ? "The provisioner builds the whole deployment for you — container, game install, agent."
                  : "No provisioner is configured, so this registers the server and generates a stack to deploy by hand."}
              </p>
            </button>

            <button
              type="button"
              onClick={() => setStage("manual")}
              className="w-full rounded-xl border border-ink/10 px-4 py-3 text-left transition hover:bg-ink/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-red/40"
            >
              <p className="text-sm font-semibold text-ink/80">Add an existing server by hand</p>
              <p className="mt-1 text-xs text-muted-foreground">
                It already runs somewhere Palcon doesn't manage — type in how to reach it.
              </p>
            </button>
          </div>
        </DialogContent>
      </Dialog>

      <ProvisionServerDialog open={open && stage === "provision"} onOpenChange={closeFrom} />
      <ServerFormDialog open={open && stage === "manual"} onOpenChange={closeFrom} mode="create" />
    </>
  );
}
