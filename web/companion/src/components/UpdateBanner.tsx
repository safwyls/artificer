import { useEffect, useState } from "react";
import { ArrowUpCircle } from "lucide-react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { shellUpdater, type ShellUpdate } from "../lib/runtime";
import { useRefreshState } from "../lib/state";
import { Button } from "./ui/button";
import type { UpdateState } from "../lib/types";

/**
 * A new build, offered rather than imposed. Neither build replaces
 * itself underneath someone silently — this says what is available and
 * waits to be asked.
 *
 * **Two builds, two mechanisms, and they are genuinely different.**
 *
 * The browser-and-tray companion is a single exe a player keeps wherever
 * they like, and it replaces itself: the daemon downloads a new binary
 * over the old one and restarts. That is what `update` here describes,
 * and it arrives on the poll like everything else.
 *
 * This app is installed, so the release is an *installer* and the thing
 * being replaced is the whole application — the daemon is one file
 * inside it. The shell owns that (electron-updater), so its status is
 * pushed from the shell rather than polled from the daemon, and the
 * daemon does not watch for updates at all. One owner, one answer.
 *
 * Both say "a different build" rather than "a newer version" on purpose:
 * every release is stamped with a commit SHA, SHAs have no order, and
 * the honest question is whether the release ships the build you are
 * running, not whether its number is bigger.
 */
export function UpdateBanner({ update }: { update: UpdateState | undefined }) {
  const shell = shellUpdater();
  return shell ? <ShellBanner shell={shell} /> : <DaemonBanner update={update} />;
}

/** The installed app: the shell checks, downloads and installs. */
function ShellBanner({ shell }: { shell: NonNullable<ReturnType<typeof shellUpdater>> }) {
  const [status, setStatus] = useState<ShellUpdate>({ state: "idle" });

  useEffect(() => {
    // Ask once for where things stand — a page that just loaded has
    // missed every event so far — then follow the pushes.
    void shell.status().then(setStatus);
    return shell.subscribe(setStatus);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Nothing to say unless there is something to do about it. "Up to
  // date" is not news, and a build that cannot update itself does not
  // need to announce that on every screen — Diagnostics carries it.
  if (!["available", "downloading", "ready", "error"].includes(status.state)) return null;
  // A failed background check is not the player's problem; a failure
  // that happened around an update they were told about is.
  if (status.state === "error" && !status.version) return null;

  const busy = status.state === "downloading";
  const act = async () => {
    try {
      if (status.state === "ready") {
        // Installs and relaunches. This window is going away, so there
        // is nothing to reload and nothing to report afterwards.
        await shell.install();
        return;
      }
      await shell.download();
    } catch (err) {
      toast.error(errorText(err));
    }
  };

  return (
    <Banner
      version={status.version}
      detail={
        status.state === "downloading"
          ? ` — downloading${status.percent ? `, ${status.percent}%` : ""}…`
          : status.state === "error"
            ? ` — ${status.why ?? "the update could not be downloaded"}`
            : " — it installs over this one and reopens."
      }
      action={
        status.state === "ready" ? "Install and reopen" : busy ? "Downloading…" : "Download update"
      }
      disabled={busy}
      onAct={act}
    />
  );
}

/** The browser build: the daemon replaces its own exe and restarts. */
function DaemonBanner({ update }: { update: UpdateState | undefined }) {
  const refresh = useRefreshState();
  const [applying, setApplying] = useState(false);
  if (!update?.available) return null;

  const apply = async () => {
    setApplying(true);
    try {
      await api.applyUpdate();
      // The companion answers and *then* restarts, so this is the last
      // thing this page hears from that process. Reloading walks into a
      // closed port; wait for the replacement to bind it.
      toast.success("updated — the companion is restarting");
      setTimeout(() => window.location.reload(), 2500);
    } catch (err) {
      toast.error(errorText(err));
      setApplying(false);
      refresh();
    }
  };

  const working = applying || Boolean(update.applying);
  return (
    <Banner
      version={update.version}
      // Offering a button that cannot work is worse than saying why.
      detail={update.supported ? " — it replaces this one and restarts." : ` ${update.why ?? ""}`}
      action={update.supported ? (working ? "Updating…" : "Update now") : undefined}
      disabled={working}
      onAct={apply}
    />
  );
}

function Banner({
  version,
  detail,
  action,
  disabled,
  onAct,
}: {
  version?: string;
  detail: string;
  action?: string;
  disabled?: boolean;
  onAct: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-panel border border-gold/50 bg-[#23180c] px-5 py-3">
      <ArrowUpCircle className="h-5 w-5 flex-none text-gold" strokeWidth={1.4} aria-hidden />
      <div className="flex-1 text-[13px]">
        <span className="text-[14px] text-parchment">
          A different companion build is available.
        </span>{" "}
        {version ? <span className="font-mono text-mist">{version}</span> : null}
        <span className="text-mist">{detail}</span>
      </div>
      {action ? (
        <Button variant="primary" onClick={onAct} disabled={disabled}>
          {action}
        </Button>
      ) : null}
    </div>
  );
}
