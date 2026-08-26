import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import {
  getAutostart,
  getStartMinimized,
  setAutostart,
  setStartMinimized,
} from "../lib/runtime";
import { useRefreshState, useSeededField } from "../lib/state";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import type { CompanionState } from "../lib/types";

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-panel border border-edge bg-panel px-6 py-5">
      <h2 className="text-[12px] uppercase tracking-[0.12em] text-gold">{title}</h2>
      <div className="mt-3 flex flex-col gap-2">{children}</div>
    </section>
  );
}

/**
 * Settings, and the drain at the bottom of the window: every fact about
 * this machine that is worth having in a bug report and worth nobody's
 * attention the rest of the time.
 *
 * Settings holds settings, and nothing else. The scan trail, the tried
 * paths and the build versions are not settings — nothing there is a
 * thing you change — so they are a dialog the status bar opens
 * (DiagnosticsDialog), not a section at the bottom of this page that a
 * link had to scroll you to.
 */
export function SettingsTab({
  state,
}: {
  state: CompanionState;
}) {
  const refresh = useRefreshState();
  const url = useSeededField(state.config?.serverUrl ?? "");
  const steam = useSeededField(state.config?.steamDirs?.[0] ?? "");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [autostart, setAutostartState] = useState<boolean | undefined>(undefined);
  const [minimized, setMinimizedState] = useState<boolean | undefined>(undefined);
  const launchOnCheckout = state.config?.launchOnCheckout ?? true;

  // Only the shell can answer this, and only some shells can. `undefined`
  // keeps the switch off the screen entirely rather than drawing one that
  // does nothing in the browser build.
  useEffect(() => {
    let live = true;
    getAutostart().then((v) => live && setAutostartState(v));
    getStartMinimized().then((v) => live && setMinimizedState(v));
    return () => {
      live = false;
    };
  }, []);


  const connect = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      // A completed connection is proven with a status poll on the
      // companion's side: a typo'd token fails here, not silently every
      // minute forever. An empty token keeps the saved one.
      await api.setConfig({ serverUrl: url.value, token });
      setToken("");
      url.settle();
      toast.success("connected");
    } catch (err) {
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


  return (
    <div className="flex max-w-[720px] flex-col gap-4 px-7 pb-6 pt-5">
      <Card title="Your vault">
        <form onSubmit={connect} className="flex flex-col gap-2">
          <Label htmlFor="settings-url">Save-sync service URL</Label>
          <Input id="settings-url" placeholder="https://vault.example.com" {...url.props} />
          <Label htmlFor="settings-token" className="mt-1">
            Your sync token {state.config?.tokenSet ? "(saved — paste to replace)" : ""}
          </Label>
          <Input
            id="settings-token"
            type="password"
            placeholder="paste the token from the service's page"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <div className="mt-2">
            <Button type="submit" variant="primary" disabled={busy}>
              Save &amp; connect
            </Button>
          </div>
        </form>
      </Card>

      <Card title="Finding your games">
        <Label htmlFor="settings-steam">Steam folder (blank = auto-detect)</Label>
        <Input
          id="settings-steam"
          className="font-mono text-[12px]"
          placeholder="e.g. D:\SteamLibrary or D:\Steam\steamapps\common"
          {...steam.props}
        />
        <p className="text-[12px] italic text-mist">
          Paste the Steam root, steamapps, or steamapps\common — extra libraries on other drives are
          found from it.
        </p>
        <div>
          <Button type="button" disabled={busy} onClick={saveSteam}>
            Save folder &amp; rescan
          </Button>
        </div>
      </Card>

      <Card title="This machine">
        <label className="flex items-start gap-2.5 text-[14px]">
          <input
            type="checkbox"
            className="mt-1"
            checked={launchOnCheckout}
            onChange={async (e) => {
              try {
                await api.setConfig({ launchOnCheckout: e.target.checked });
              } catch (err) {
                toast.error(errorText(err));
              } finally {
                refresh();
              }
            }}
          />
          <span>
            Start the game when I check a world out
            <span className="mt-0.5 block text-[12px] italic text-mist">
              The save is put in place first, then the game starts — never the other way round.
              Switch it off to take custody of a world without opening it. Games linked by hand carry
              nothing that says what starts them, so those check out without launching either way.
            </span>
          </span>
        </label>
        {autostart !== undefined ? (
          <label className="mt-2 flex items-start gap-2.5 text-[14px]">
            <input
              type="checkbox"
              className="mt-1"
              checked={autostart}
              onChange={async (e) => {
                const want = e.target.checked;
                setAutostartState(want);
                if (!(await setAutostart(want))) {
                  setAutostartState(!want);
                  toast.error("this build could not change the autostart setting");
                  return;
                }
                // Read it back rather than trust the write. The setting
                // lives in the OS, not in this page, and a switch that
                // shows what was *asked for* is how this one came to
                // look like it worked when it did not.
                const actual = await getAutostart();
                if (actual !== undefined) setAutostartState(actual);
              }}
            />
            <span>
              Start the companion when I sign in
              <span className="mt-0.5 block text-[12px] italic text-mist">
                A hold you forgot about expires whether the companion is running or not — but only a
                running companion can warn you first.
              </span>
            </span>
          </label>
        ) : null}

        {minimized !== undefined ? (
          <label className="mt-2 flex items-start gap-2.5 text-[14px]">
            <input
              type="checkbox"
              className="mt-1"
              checked={minimized}
              onChange={async (e) => {
                const want = e.target.checked;
                setMinimizedState(want);
                if (!(await setStartMinimized(want))) {
                  setMinimizedState(!want);
                  toast.error("this build could not change that setting");
                  return;
                }
                const actual = await getStartMinimized();
                if (actual !== undefined) setMinimizedState(actual);
              }}
            />
            <span>
              Start minimized to the tray
              <span className="mt-0.5 block text-[12px] italic text-mist">
                Signing in already opens it quietly. This is for the rest of the time — the
                companion is a thing that runs, not a thing you look at.
              </span>
            </span>
          </label>
        ) : null}
      </Card>

    </div>
  );
}
