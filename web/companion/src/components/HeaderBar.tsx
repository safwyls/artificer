import { useEffect, useState } from "react";
import { Laptop, RefreshCw, Settings } from "lucide-react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { freshness } from "../lib/format";
import { useRefreshState } from "../lib/state";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";
import type { CompanionState } from "../lib/types";

/**
 * The connection, said once at the top instead of in a panel at the
 * bottom: whether the vault is reachable, as whom, how old what you are
 * looking at is, and the two things you might want to do about it.
 */
export function HeaderBar({
  state,
  onOpenSettings,
}: {
  state: CompanionState | undefined;
  onOpenSettings: () => void;
}) {
  const refresh = useRefreshState();
  const [syncing, setSyncing] = useState(false);
  const sync = state?.sync;
  const configured = Boolean(sync?.configured);

  // Freshness is an age, so it has to be recomputed on a clock of its own
  // — the poll it describes may answer with an unchanged timestamp.
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);

  // Asking now rather than waiting for the poll. The page keeps itself
  // current while it is open, so this is for being certain rather than
  // patient — and for hearing plainly when the service cannot be reached,
  // which a background poll never says out loud.
  const syncNow = async () => {
    setSyncing(true);
    try {
      const out = await api.syncNow();
      toast.success(`synced — ${out.worlds} world${out.worlds === 1 ? "" : "s"} on the service`);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setSyncing(false);
      refresh();
    }
  };

  return (
    <header className="flex flex-wrap items-center gap-3.5 border-b border-edge bg-well px-7 py-4">
      <Laptop className="h-6 w-6 flex-none text-gold" strokeWidth={1.3} aria-hidden />
      <div>
        <div className="text-[18px] tracking-[0.05em] text-gold">Artificer Companion</div>
        <div className="text-[11px] text-mist">shared world saves, synced from this machine</div>
      </div>
      <div className="ml-auto flex flex-wrap items-center gap-3">
        <span className="inline-flex items-center gap-1.5 text-[13px]">
          <span
            className={cn(
              "inline-block h-[7px] w-[7px] rounded-full",
              sync?.lastError ? "bg-ember" : configured ? "bg-ok" : "bg-mist",
            )}
            aria-hidden
          />
          {configured ? (
            <>
              Connected as <b className="ml-1">{sync?.username ?? "…"}</b>
            </>
          ) : (
            <span className="text-mist">Not connected</span>
          )}
        </span>
        {configured ? (
          <span className="font-mono text-[12px] text-mist">
            {sync?.busy ? "transfer in progress…" : freshness(sync?.polledAt)}
          </span>
        ) : null}
        {configured ? (
          <Button onClick={syncNow} disabled={syncing}>
            <RefreshCw className={cn("h-3.5 w-3.5", syncing && "animate-spin")} aria-hidden />
            {syncing ? "Syncing…" : "Sync now"}
          </Button>
        ) : null}
        <Button size="icon" onClick={onOpenSettings} aria-label="Settings">
          <Settings className="h-3.5 w-3.5" aria-hidden />
        </Button>
      </div>
      {sync?.lastError ? (
        <div className="w-full text-[13px] text-ember">{sync.lastError}</div>
      ) : null}
    </header>
  );
}

/**
 * Both builds, side by side: which companion, and which service it is
 * talking to. A save-sync report that names one half names nothing.
 */
export function FooterBar({ state }: { state: CompanionState | undefined }) {
  const server = state?.sync?.serverVersion;
  const versions =
    `companion ${state?.version || "dev"}` +
    (server
      ? ` · service ${server}`
      : state?.sync?.configured
        ? " · service version unknown"
        : "");
  return (
    <footer className="flex flex-wrap items-center gap-2.5 border-t border-edge px-7 py-2.5 font-mono text-[11px] text-mist">
      <span>{versions}</span>
      {state?.sync?.lastAction ? (
        <span className="ml-auto">last action: {state.sync.lastAction}</span>
      ) : null}
    </footer>
  );
}
