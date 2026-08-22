import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { useRefreshState, useSeededField } from "../lib/state";
import { ScanTrail } from "./ScanTrail";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import type { CompanionState } from "../lib/types";

/**
 * Setup is a deliberate state, not an empty page with forms at the
 * bottom. Until this companion is connected, the two things that have to
 * be true — it can reach a vault, and it can find your games — are the
 * whole screen, side by side, with the scan trail already visible so
 * "no games found" always names its own cause.
 */
export function FirstRun({ state }: { state: CompanionState }) {
  const refresh = useRefreshState();
  const url = useSeededField(state.config?.serverUrl ?? "");
  const steam = useSeededField(state.config?.steamDirs?.[0] ?? "");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");

  const connect = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setStatus("connecting…");
    try {
      await api.setConfig({ serverUrl: url.value, token });
      setToken("");
      url.settle();
      setStatus("connected");
      toast.success("connected");
    } catch (err) {
      setStatus(errorText(err));
      toast.error(errorText(err));
    } finally {
      setBusy(false);
      refresh();
    }
  };

  const saveSteam = async () => {
    const dir = steam.value.trim();
    setBusy(true);
    try {
      await api.setConfig({ steamDirs: dir ? [dir] : [] });
      steam.settle(dir);
      toast.success(dir ? "folder saved — rescanned" : "override cleared — rescanned");
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
      refresh();
    }
  };

  const card = "flex flex-1 flex-col gap-2.5 rounded-panel border border-edge bg-panel px-6 py-6";
  const heading = "text-[12px] uppercase tracking-[0.12em] text-gold";

  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="flex w-full max-w-[860px] flex-wrap gap-6">
        <form onSubmit={connect} className={card}>
          <h2 className={heading}>Connect to your vault</h2>
          <p className="text-[13px] text-mist">
            Nothing leaves this machine until you connect. Ask whoever runs your group&apos;s sync service for the
            address, and mint your token on its page.
          </p>
          <div className="mt-1.5">
            <Label htmlFor="setup-url">Save-sync service URL</Label>
            <Input id="setup-url" className="mt-1" placeholder="https://vault.example.com" {...url.props} />
          </div>
          <div>
            <Label htmlFor="setup-token">Your sync token</Label>
            <Input
              id="setup-token"
              className="mt-1"
              type="password"
              placeholder="paste the token from the service's page"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </div>
          <div className="mt-1.5 flex items-center gap-2.5">
            <Button type="submit" variant="primary" size="lg" disabled={busy}>
              Save &amp; connect
            </Button>
            <span className="font-mono text-[12px] text-mist">
              {status || (state.config?.tokenSet ? "token saved" : "not configured")}
            </span>
          </div>
        </form>

        <div className={card}>
          <h2 className={heading}>Finding your games</h2>
          <p className="text-[13px] text-mist">
            Steam is detected automatically — the registry, then the usual install paths. Set a folder only if the
            scan misses a library.
          </p>
          <div className="mt-1.5">
            <Label htmlFor="setup-steam">Steam folder (blank = auto-detect)</Label>
            <Input
              id="setup-steam"
              className="mt-1 font-mono text-[12px]"
              placeholder="e.g. D:\SteamLibrary or D:\Steam\steamapps\common"
              {...steam.props}
            />
          </div>
          <div className="mt-1.5">
            <Button type="button" disabled={busy} onClick={saveSteam}>
              Save folder &amp; rescan
            </Button>
          </div>
          <div className="rounded border border-edge bg-ink px-2.5 py-2">
            <ScanTrail probes={state.discovered?.probes ?? []} />
          </div>
          <p className="text-[12px] italic text-mist">
            &ldquo;No games found&rdquo; always names its own cause here.
          </p>
        </div>
      </div>
    </div>
  );
}
