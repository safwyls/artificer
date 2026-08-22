import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { useRefreshState } from "../lib/state";
import { custodyOf, type Artwork, type Link, type SyncWorld } from "../lib/types";
import { CoverArt } from "./CoverArt";
import { CustodyChip, custodyLine } from "./CustodyChip";
import { ConfirmDialog } from "./ConfirmDialog";
import { Button } from "./ui/button";

/**
 * One linked world: what it is, who holds it, the folder on this machine,
 * and the single action its custody state calls for. These verbs and
 * their visibility are the old page's `linkedRow()`, rule for rule —
 * including that a hold belonging to this account but to a *different*
 * session offers nothing, because the download is still on its way here.
 */
export function WorldRow({
  link,
  world,
  me,
  art,
  configured,
}: {
  link: Link;
  world: SyncWorld | undefined;
  me: string | undefined;
  art: Record<string, Artwork>;
  configured: boolean;
}) {
  const refresh = useRefreshState();
  const custody = custodyOf(link, world, me, configured);
  const title = link.gameTitle || world?.world.name || "";

  const run = async (fn: () => Promise<unknown>, okMsg?: string) => {
    try {
      await fn();
      if (okMsg) toast.success(okMsg);
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      refresh();
    }
  };

  return (
    <div className="flex items-start gap-3.5 rounded-panel border border-edge bg-panel px-4 py-3.5">
      <CoverArt art={art} game={{ appId: link.appId, name: title }} variant="thumb" />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="text-[16px] font-bold">
            {world ? world.world.name : `world #${link.worldId}`}
          </span>
          {link.gameTitle ? <span className="text-[12px] text-rune">{link.gameTitle}</span> : null}
          <CustodyChip custody={custody} className="ml-auto" />
        </div>
        <div className="text-[13px] text-mist">{custodyLine(custody, link, world, me)}</div>
        {/* The folder, in full: the one thing on this page that is about
            this machine rather than the world, and the thing a player
            checks when a save goes to the wrong place. */}
        <div className="break-all font-mono text-[11px] text-mist">{link.dir}</div>
        <div className="flex flex-wrap gap-2">
          {custody === "free" ? (
            <Button
              variant="primary"
              onClick={() =>
                run(() => api.checkout(link.worldId, false), "checked out — the save is on this machine")
              }
            >
              Check out &amp; host
            </Button>
          ) : null}
          {custody === "mine" ? (
            <>
              <Button
                variant="primary"
                onClick={() => run(() => api.checkin(link.worldId), "checked in — the world is free")}
              >
                Check in
              </Button>
              {/* A checkpoint never moves the head; the service only keeps
                  them for worlds that asked for them. */}
              {world?.world.checkpoints ? (
                <Button onClick={() => run(() => api.checkpoint(link.worldId), "checkpoint pushed")}>
                  Checkpoint now
                </Button>
              ) : null}
              <Button onClick={() => run(() => api.renew(link.worldId), "hold renewed")}>
                Renew hold
              </Button>
            </>
          ) : null}
          {custody === "expired" ? (
            <ConfirmDialog
              trigger={<Button variant="primary">Take over expired hold</Button>}
              title="Take over the expired hold?"
              body="The old holder's late check-in is kept and flagged, not lost."
              confirmLabel="Take over"
              onConfirm={() => run(() => api.checkout(link.worldId, true), "world taken over")}
            />
          ) : null}
          {(custody === "held" || custody === "expired") && !world?.claimedBy ? (
            <Button
              onClick={() =>
                run(
                  () => api.claim(link.worldId),
                  "you're next — the world downloads automatically when it frees up",
                )
              }
            >
              Claim next
            </Button>
          ) : null}
          <ConfirmDialog
            trigger={<Button>Unlink</Button>}
            title="Unlink this world from its folder?"
            body="Nothing is deleted."
            confirmLabel="Unlink"
            onConfirm={() => run(() => api.unlink(link.worldId), "unlinked")}
          />
        </div>
      </div>
    </div>
  );
}
