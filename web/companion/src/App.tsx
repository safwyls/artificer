import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api, errorText } from "./lib/api";
import { cn } from "./lib/utils";
import { useLiveUpdates } from "./lib/events";
import { useArtwork, useCompanionState, useHistory, useRefreshState, useSaveHints } from "./lib/state";
import { ActivityTab } from "./components/ActivityTab";
import { ConflictsTab, conflictCount } from "./components/ConflictsTab";
import { FirstRun } from "./components/FirstRun";
import { StatusBar } from "./components/StatusBar";
import { LinkGameDialog, byHandGame } from "./components/LinkGameDialog";
import { LinkedGameDialog } from "./components/LinkedGameDialog";
import { NoWorlds } from "./components/NoWorlds";
import { PanelBoundary } from "./components/Panel";
import { SettingsTab } from "./components/SettingsTab";
import { TabBar, type Tab } from "./components/TabBar";
import { TitleBar } from "./components/TitleBar";
import { Button } from "./components/ui/button";
import { UpdateBanner } from "./components/UpdateBanner";
import { GamesTab, linkFor } from "./components/GamesTab";
import { WorldsTab } from "./components/WorldsTab";
import { tileKey } from "./components/GameTile";
import type { DiscoveredGame } from "./lib/types";

export function App() {
  const refresh = useRefreshState();
  // The push stream first: while it is carrying, the poll drops to a
  // heartbeat. It never stops — a dropped stream is invisible from both
  // ends, and custody is the wrong thing to be quietly wrong about.
  const live = useLiveUpdates();
  const { data: state, isLoading, isError, error } = useCompanionState(live);
  const games = state?.discovered?.games ?? [];
  // Both of these ask when the *set* of games changes, never on the poll.
  const { art, empty: artEmpty, error: artError } = useArtwork(games);
  const hints = useSaveHints(games, Boolean(state?.sync?.configured));

  /** Which shelf entry is open, if any. Held here rather than in the
   * shelf so a poll that rebuilds tiles cannot close it. */
  const [open, setOpen] = useState<DiscoveredGame | null>(null);
  const [tab, setTab] = useState<Tab>("worlds");
  const [toDiagnostics, setToDiagnostics] = useState(false);
  const [syncing, setSyncing] = useState(false);

  // The vault's record of what happened, read only while one of the two
  // tabs that shows it is open: one request per linked world, and neither
  // view is needed to sync a save.
  const history = useHistory(
    Boolean(state?.sync?.configured) && (tab === "activity" || tab === "conflicts"),
  );

  // Both of these keep the titlebar: it is the only thing the shell's
  // frameless window can be dragged by, and a window you cannot move is
  // the worst place to be told the companion is not answering.
  if (isLoading) {
    return (
      <div className="flex h-screen flex-col">
        <TitleBar />
        <p className="p-8 text-mist">Reading this machine…</p>
      </div>
    );
  }
  if (isError || !state) {
    return (
      <div className="flex h-screen flex-col">
        <TitleBar />
        <p className="p-8 font-mono text-[13px] text-ember">
          The companion is not answering on this machine: {errorText(error)}
        </p>
      </div>
    );
  }

  const links = state.links ?? [];
  const worlds = state.sync?.worlds ?? [];
  const openLink = open ? linkFor(open, links) : undefined;
  const configured = Boolean(state.sync?.configured);
  // Offline is derived from connectivity, never chosen: the companion's
  // last attempt to reach the vault either worked or said why it did not.
  // First run is derived the same way, from "no linked worlds" — so it
  // comes back on its own if every link is removed.
  const offline = configured && Boolean(state.sync?.lastError);

  const goTab = (t: Tab) => {
    setToDiagnostics(false);
    setTab(t);
  };

  const rescan = async () => {
    try {
      const out = await api.discover();
      toast.success(`rescanned — ${out.found} game${out.found === 1 ? "" : "s"} found`);
    } catch (err) {
      toast.error(errorText(err));
    }
    refresh();
  };

  // Asking now rather than waiting for the poll: for being certain rather
  // than patient, and for hearing plainly when the vault cannot be
  // reached, which a background poll never says out loud.
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
    // A fixed frame with one scrolling region in the middle, rather than
    // one long scrolling page. Under the shell the titlebar has to stay
    // put to stay draggable, and the status bar is the same promise the
    // header makes — both belong to the window, not to the content.
    <div className="flex h-screen flex-col overflow-hidden">
      <TitleBar state={state} syncing={syncing} />
      <TabBar
        tab={tab}
        onTab={goTab}
        conflicts={conflictCount(history.history)}
        actions={
          // The window's one action, and only once there is a vault to
          // sync with. It asks now rather than waiting for the poll: for
          // being certain rather than patient.
          //
          // Frameless and wordless: it sits on the tab row's rule, where
          // a bordered box reads as a second piece of chrome, and what it
          // would have said is already said — the titlebar carries the
          // sync state in words a few pixels above it, and turns this
          // same arrow while a sync runs. The name survives as the
          // accessible label and the tooltip, which is where a control
          // with no text has to keep it.
          configured ? (
            <Button
              variant="bare"
              size="icon"
              onClick={syncNow}
              disabled={syncing}
              aria-label={syncing ? "Syncing…" : "Sync now"}
              title={syncing ? "Syncing…" : "Sync now"}
            >
              <RefreshCw className={cn("h-4 w-4", syncing && "animate-spin")} aria-hidden />
            </Button>
          ) : null
        }
      />

      <main className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        {!configured ? (
          <PanelBoundary name="setup">
            <div className="flex flex-1 flex-col">
              {state.update?.available ? (
                <div className="px-7 pt-6">
                  <UpdateBanner update={state.update} />
                </div>
              ) : null}
              <FirstRun state={state} />
            </div>
          </PanelBoundary>
        ) : tab === "worlds" ? (
          <>
            {state.update?.available ? (
              <div className="px-7 pt-5">
                <PanelBoundary name="update">
                  <UpdateBanner update={state.update} />
                </PanelBoundary>
              </div>
            ) : null}
            <PanelBoundary name="worlds">
              {links.length ? (
                <WorldsTab
                  state={state}
                  art={art}
                  offline={offline}
                  retrying={syncing}
                  onRetry={syncNow}
                  onOpenGames={() => goTab("games")}
                />
              ) : (
                <NoWorlds state={state} onOpenGames={() => goTab("games")} />
              )}
            </PanelBoundary>
          </>
        ) : tab === "games" ? (
          <PanelBoundary name="games">
            <GamesTab
                state={state}
                art={art}
                artEmpty={artEmpty}
                artError={artError}
                hints={hints}
                activeKey={open ? tileKey(open) : null}
                onOpen={setOpen}
                onRescan={rescan}
              onLinkByHand={() => setOpen(byHandGame())}
            />
          </PanelBoundary>
        ) : tab === "activity" ? (
          <PanelBoundary name="activity">
            <ActivityTab
              history={history.history}
              loading={history.loading}
              error={history.error}
              refreshing={history.refreshing}
              onRefresh={history.refresh}
            />
          </PanelBoundary>
        ) : tab === "conflicts" ? (
          <PanelBoundary name="conflicts">
            <ConflictsTab
              history={history.history}
              loading={history.loading}
              error={history.error}
              refreshing={history.refreshing}
              onRefresh={history.refresh}
              serverUrl={state.config?.serverUrl}
            />
          </PanelBoundary>
        ) : (
          <PanelBoundary name="settings">
            <SettingsTab state={state} focusDiagnostics={toDiagnostics} />
          </PanelBoundary>
        )}
      </main>

      <StatusBar
        state={state}
        onDiagnostics={() => {
          setTab("settings");
          setToDiagnostics(true);
        }}
      />

      {/* A linked entry opens what it points at; an unlinked one opens the
          link form. Both are dialogs, so the poll under them is free to
          rebuild the shelf. */}
      {open && openLink ? (
        <LinkedGameDialog
          game={open}
          link={openLink}
          world={worlds.find((w) => w.world.id === openLink.worldId)}
          art={art}
          onClose={() => setOpen(null)}
        />
      ) : null}
      {open && !openLink ? (
        <LinkGameDialog game={open} state={state} art={art} onClose={() => setOpen(null)} />
      ) : null}
    </div>
  );
}
