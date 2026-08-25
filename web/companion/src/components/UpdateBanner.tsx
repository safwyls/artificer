import { useState } from "react";
import { ArrowUpCircle } from "lucide-react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { inShell, runInstaller } from "../lib/runtime";
import { useRefreshState } from "../lib/state";
import { Button } from "./ui/button";
import type { UpdateState } from "../lib/types";

/**
 * A new build, offered rather than imposed. The companion checks GitHub
 * on its own, but replacing a binary underneath someone is not something
 * to do silently — so this says what is available and waits to be asked.
 *
 * It says "a different build" rather than "a newer version" on purpose:
 * every release is stamped with a commit SHA, SHAs have no order, and
 * the honest question is whether the release ships the build you are
 * running, not whether its number is bigger.
 */
export function UpdateBanner({ update }: { update: UpdateState | undefined }) {
  const refresh = useRefreshState();
  const [applying, setApplying] = useState(false);
  if (!update?.available) return null;

  const apply = async () => {
    setApplying(true);
    try {
      const out = await api.applyUpdate();
      // Two shapes of update, and the daemon says which by whether it
      // hands back an installer.
      //
      // The browser build replaces its own exe and restarts, so the
      // answer above is the last thing this page hears from that process
      // — reloading immediately walks into a closed port.
      //
      // This one downloaded an installer instead, because the release
      // installs an application rather than replacing one file. Running
      // it means quitting the app it replaces, which only the shell can
      // do; there is nothing to reload afterwards, because the window is
      // going away.
      if (out.installer) {
        if (await runInstaller(out.installer)) {
          toast.success("installing — the companion will close");
          return;
        }
        // Downloaded and verified, but nothing here can run it. Say
        // where it is rather than swallow the work.
        toast.error(`the update was downloaded but could not be started: ${out.installer}`);
        setApplying(false);
        refresh();
        return;
      }
      toast.success("updated — the companion is restarting");
      setTimeout(() => window.location.reload(), 2500);
    } catch (err) {
      toast.error(errorText(err));
      setApplying(false);
      refresh();
    }
  };

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-panel border border-gold/50 bg-[#23180c] px-5 py-3">
      <ArrowUpCircle className="h-5 w-5 flex-none text-gold" strokeWidth={1.4} aria-hidden />
      <div className="flex-1 text-[13px]">
        <span className="text-[14px] text-parchment">A different companion build is available.</span>{" "}
        <span className="font-mono text-mist">{update.version}</span>
        {update.supported ? (
          <span className="text-mist">
            {update.installer !== undefined || inShell()
              ? " — it installs over this one and closes the app."
              : " — it replaces this one and restarts."}
          </span>
        ) : (
          // Offering a button that cannot work is worse than saying why.
          <span className="text-mist"> {update.why}</span>
        )}
      </div>
      {update.supported ? (
        <Button variant="primary" onClick={apply} disabled={applying || update.applying}>
          {applying || update.applying ? "Updating…" : "Update now"}
        </Button>
      ) : null}
    </div>
  );
}
