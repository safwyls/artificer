import { useState } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { shellUpdater } from "../lib/runtime";
import { ScanTrail } from "./ScanTrail";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import type { CompanionState } from "../lib/types";

/**
 * Every fact about this machine that a bug report needs and that nobody
 * needs the rest of the time.
 *
 * A dialog rather than a section of Settings. It is not a setting —
 * nothing here is a thing you change — and burying it at the bottom of
 * the settings page meant the status bar's link had to scroll you to it
 * and leave you on a tab you did not ask for. It opens over whatever you
 * were looking at and closes back onto it.
 *
 * It lives in exactly one place, which is the same rule that took the
 * settings cog off the header: Settings holds settings, and this holds
 * the drain.
 */
export function DiagnosticsDialog({
  state,
  onClose,
}: {
  state: CompanionState;
  onClose: () => void;
}) {
  const [checking, setChecking] = useState(false);
  const shell = shellUpdater();
  const probes = state.discovered?.probes ?? [];
  const links = state.links ?? [];
  const update = state.update;

  // Ask whichever updater this build actually has.
  //
  // This used to always ask the daemon's, which inside the shell is the
  // wrong product entirely: companiond does not watch for updates here,
  // and its checker points at `companion-latest` — the browser-and-tray
  // build's release track. Clicking this reported that a different
  // build of *that* was available, which is true and useless.
  const checkUpdate = async () => {
    setChecking(true);
    try {
      if (shell) {
        const status = await shell.check();
        if (status.state === "error") toast.error(status.why ?? "the check failed");
        else if (status.state === "available" || status.state === "ready") {
          toast.success(`update available: ${status.version ?? "a different build"}`);
        } else if (status.state === "unsupported") toast.info(status.why ?? "this build cannot update itself");
        else toast.success("up to date");
      } else {
        const { update: u } = await api.checkUpdate();
        if (u?.error) toast.error(u.error);
        else if (u?.available) toast.success(`update available: ${u.version}`);
        else toast.success("up to date");
      }
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setChecking(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      {/* Wider and scrollable: a scan trail is long, and a save path is
          not something to wrap into six lines. */}
      <DialogContent className="max-h-[80vh] max-w-2xl overflow-y-auto">
        <DialogTitle>Diagnostics</DialogTitle>
        <p className="mt-2 text-[12.5px] text-mist">
          Nothing here is needed to use the companion. It is what a bug report needs.
        </p>

        <div className="mt-3 font-mono text-[12px] text-parchment/80">
          companion {state.version || "dev"}
          {state.sync?.serverVersion
            ? ` · service ${state.sync.serverVersion}`
            : state.sync?.configured
              ? " · service version unknown"
              : ""}
        </div>
        {state.hostname ? (
          <div className="font-mono text-[12px] text-parchment/80">machine: {state.hostname}</div>
        ) : null}
        {state.sync?.lastAction ? (
          <div className="text-[12px] text-mist">last action: {state.sync.lastAction}</div>
        ) : null}
        {state.sync?.lastError ? (
          <div className="text-[12px] text-ember">last error: {state.sync.lastError}</div>
        ) : null}

        <div className="mt-3 rounded border border-edge bg-ink px-2.5 py-2">
          <ScanTrail probes={probes} />
          {probes.length ? null : (
            <p className="font-mono text-[12px] text-mist">no scan has run yet</p>
          )}
        </div>

        <div className="mt-3">
          <div className="text-[11px] uppercase tracking-[0.1em] text-mist">
            Linked save folders
          </div>
          {links.length ? (
            <ul className="mt-1 flex flex-col gap-1">
              {links.map((l) => (
                <li key={l.worldId} className="break-all font-mono text-[12px] text-mist">
                  #{l.worldId} → {l.dir}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-1 text-[12px] text-mist">nothing linked on this machine</p>
          )}
        </div>

        <div className="mt-4">
          <p className="text-[12px] italic text-mist">
            {shell
              ? "this app checks for its own updates shortly after it starts"
              : update?.error
                ? `last update check failed: ${update.error}`
                : update?.available
                  ? `a different build is available: ${update.version}`
                  : update?.checkedAt
                    ? "up to date, as of the last check"
                    : "checked automatically every few hours"}
          </p>
          <Button type="button" className="mt-2" disabled={checking} onClick={checkUpdate}>
            Check for update
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
