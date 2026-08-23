import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { getAutostart, setAutostart } from "../lib/runtime";
import { useRefreshState, useSeededField } from "../lib/state";
import { ScanTrail } from "./ScanTrail";
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
 * The scan trail, the tried paths and the two build versions used to sit
 * in the page chrome, where they competed with the worlds for the eye and
 * said the sync state a second and third time. They are diagnostics, so
 * they live under a Diagnostics heading, and the footer link comes here.
 */
export function SettingsTab({
  state,
  focusDiagnostics,
}: {
  state: CompanionState;
  /** Set when the player arrived by way of the footer's Diagnostics link,
   * so the page opens where they were going rather than at the top. */
  focusDiagnostics?: boolean;
}) {
  const refresh = useRefreshState();
  const url = useSeededField(state.config?.serverUrl ?? "");
  const steam = useSeededField(state.config?.steamDirs?.[0] ?? "");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [autostart, setAutostartState] = useState<boolean | undefined>(undefined);
  const diagnostics = useRef<HTMLDivElement | null>(null);
  const launchOnCheckout = state.config?.launchOnCheckout ?? true;
  const update = state.update;

  // Only the shell can answer this, and only some shells can. `undefined`
  // keeps the switch off the screen entirely rather than drawing one that
  // does nothing in the browser build.
  useEffect(() => {
    let live = true;
    getAutostart().then((v) => live && setAutostartState(v));
    return () => {
      live = false;
    };
  }, []);

  useEffect(() => {
    if (focusDiagnostics) diagnostics.current?.scrollIntoView({ block: "start" });
  }, [focusDiagnostics]);

  const checkUpdate = async () => {
    setCheckingUpdate(true);
    try {
      const { update: u } = await api.checkUpdate();
      if (u?.error) toast.error(u.error);
      else if (u?.available) toast.success(`update available: ${u.version}`);
      else toast.success("you're up to date");
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setCheckingUpdate(false);
      refresh();
    }
  };

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

  const links = state.links ?? [];
  const probes = state.discovered?.probes ?? [];

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
                }
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
      </Card>

      <div ref={diagnostics} className="scroll-mt-4">
        <Card title="Diagnostics">
          <p className="text-[12.5px] text-mist">
            Nothing here is needed to use the companion. It is what a bug report needs.
          </p>

          <div className="mt-1 font-mono text-[11px] text-mist">
            companion {state.version || "dev"}
            {state.sync?.serverVersion
              ? ` · service ${state.sync.serverVersion}`
              : state.sync?.configured
                ? " · service version unknown"
                : ""}
          </div>
          {state.sync?.lastAction ? (
            <div className="font-mono text-[11px] text-mist">
              last action: {state.sync.lastAction}
            </div>
          ) : null}
          {state.sync?.lastError ? (
            <div className="font-mono text-[11px] text-ember">
              last error: {state.sync.lastError}
            </div>
          ) : null}

          <div className="mt-2 rounded border border-edge bg-ink px-2.5 py-2">
            <ScanTrail probes={probes} />
            {probes.length ? null : (
              <p className="font-mono text-[12px] text-mist">no scan has run yet</p>
            )}
          </div>

          <div className="mt-2">
            <div className="text-[11px] uppercase tracking-[0.1em] text-mist">Linked save folders</div>
            {links.length ? (
              <ul className="mt-1 flex flex-col gap-1">
                {links.map((l) => (
                  <li key={l.worldId} className="break-all font-mono text-[11px] text-mist">
                    #{l.worldId} → {l.dir}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-1 font-mono text-[11px] text-mist">nothing linked on this machine</p>
            )}
          </div>

          <div className="mt-3">
            <p className="text-[12px] italic text-mist">
              {update?.error
                ? `last update check failed: ${update.error}`
                : update?.available
                  ? `a different build is available: ${update.version}`
                  : update?.checkedAt
                    ? "up to date, as of the last check"
                    : "checked automatically every few hours"}
            </p>
            <Button type="button" className="mt-2" disabled={checkingUpdate} onClick={checkUpdate}>
              Check for update
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
