import { useState } from "react";
import { toast } from "sonner";
import { api, errorText } from "./lib/api";
import { useArtwork, useCompanionState, useRefreshState, useSaveHints } from "./lib/state";
import { FirstRun } from "./components/FirstRun";
import { FooterBar, HeaderBar } from "./components/HeaderBar";
import { LinkGameDialog, byHandGame } from "./components/LinkGameDialog";
import { LinkedGameDialog } from "./components/LinkedGameDialog";
import { PanelBoundary, SectionHeader } from "./components/Panel";
import { SettingsTab } from "./components/SettingsTab";
import { TabBar, type Tab } from "./components/TabBar";
import { UpdateBanner } from "./components/UpdateBanner";
import { Shelf, linkFor } from "./components/Shelf";
import { WorldRow } from "./components/WorldRow";
import { tileKey } from "./components/GameTile";
import type { CompanionState, DiscoveredGame } from "./lib/types";

/**
 * A tab with no backing surface yet. It names where the ability actually
 * lives rather than drawing an empty list that looks broken — the same
 * rule the consoles follow when a game cannot support a feature.
 */
function NotHereYet({ title, body }: { title: string; body: string }) {
  return (
    <div className="flex flex-col gap-2 px-7 pb-6 pt-6">
      <SectionHeader title={title} />
      <p className="max-w-[62ch] text-[13px] text-mist">{body}</p>
    </div>
  );
}

export function App() {
  const refresh = useRefreshState();
  const { data: state, isLoading, isError, error } = useCompanionState();
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

  if (isLoading) {
    return <p className="p-8 text-mist">Reading this machine…</p>;
  }
  if (isError || !state) {
    return (
      <p className="p-8 font-mono text-[13px] text-ember">
        The companion is not answering on this machine: {errorText(error)}
      </p>
    );
  }

  const links = state.links ?? [];
  const worlds = state.sync?.worlds ?? [];
  const openLink = open ? linkFor(open, links) : undefined;
  const configured = Boolean(state.sync?.configured);

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
    <div className="flex min-h-screen flex-col">
      <HeaderBar
        state={state}
        syncing={syncing}
        onSyncNow={syncNow}
        onOpenSettings={() => goTab("settings")}
      />
      <TabBar tab={tab} onTab={goTab} />

      <main className="flex flex-1 flex-col">
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
          <div className="flex flex-col gap-5 px-7 pb-6 pt-5">
            <PanelBoundary name="update">
              <UpdateBanner update={state.update} />
            </PanelBoundary>
            <PanelBoundary name="worlds">
              <section className="flex flex-col gap-2.5">
                <SectionHeader title="Your worlds" />
                {links.length ? (
                  links.map((link) => (
                    <WorldRow
                      key={link.worldId}
                      link={link}
                      world={worlds.find((w) => w.world.id === link.worldId)}
                      me={state.sync?.username}
                      art={art}
                      configured
                      launchOnCheckout={state.config?.launchOnCheckout ?? true}
                    />
                  ))
                ) : (
                  <p className="text-[13px] italic text-mist">
                    Nothing linked yet — link an installed game from the Games tab, or ask whoever
                    runs your sync service which world to join.
                  </p>
                )}
              </section>
            </PanelBoundary>
          </div>
        ) : tab === "games" ? (
          <div className="flex flex-col gap-5 px-7 pb-6 pt-5">
            <PanelBoundary name="shelf">
              <Shelf
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
          </div>
        ) : tab === "activity" ? (
          <NotHereYet
            title="Activity"
            body={activityBody(state)}
          />
        ) : tab === "conflicts" ? (
          <NotHereYet
            title="Conflicts"
            body="A conflict is two check-ins of the same world from different machines, and the vault is the only thing that can see one. Reliquary keeps them with the world's history — this tab lights up when the companion is taught to read that list."
          />
        ) : (
          <PanelBoundary name="settings">
            <SettingsTab state={state} focusDiagnostics={toDiagnostics} />
          </PanelBoundary>
        )}
      </main>

      <FooterBar
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

/** What the companion can honestly say about recent activity today: the
 * last thing it did, and that nothing is waiting to be sent. */
function activityBody(state: CompanionState): string {
  const last = state.sync?.lastAction;
  const queued = state.sync?.queue?.length ?? 0;
  const head = last ? `The last thing this companion did: ${last}.` : "This companion has not moved a save yet this session.";
  const tail =
    queued > 0
      ? ` ${queued} transfer${queued === 1 ? "" : "s"} are waiting to be sent.`
      : " Nothing is waiting to be sent — a checkout, a checkpoint and a check-in each either reach the vault now or fail now.";
  return `${head}${tail} The full history of a world lives in the vault, beside the world itself.`;
}
