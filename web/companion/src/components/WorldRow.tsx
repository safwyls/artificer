import { useState } from "react";
import { Play } from "lucide-react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { fmtBytes, fmtWhen } from "../lib/format";
import { nativeFolders, openPath } from "../lib/runtime";
import { useRefreshState } from "../lib/state";
import { custodyOf, launchable, type Artwork, type Custody, type Link, type SyncWorld } from "../lib/types";
import { cn } from "../lib/utils";
import { CoverArt } from "./CoverArt";
import { CustodyChip, HoldCountdown, custodyLine } from "./CustodyChip";
import { ConfirmDialog } from "./ConfirmDialog";
import { EditWorldDialog } from "./EditWorldDialog";
import { OverflowMenu, type MenuItem } from "./OverflowMenu";
import { Button } from "./ui/button";

/**
 * The mono line on the right of a row: version, size, when it was last
 * written. Three machine facts, in the machine's typeface, where the eye
 * can skip them until it wants them.
 */
function headMeta({ world }: { world: SyncWorld | undefined }): string[] {
  if (!world) return [];
  const v = world.world.headVersion;
  return [
    [v ? `v${v}` : "", fmtBytes(world.head?.bytes), fmtWhen(world.head?.createdAt)]
      .filter(Boolean)
      .join(" · "),
  ].filter(Boolean);
}

/**
 * One linked world: what it is, who holds it, and the single action its
 * custody state calls for.
 *
 * The row used to carry four buttons of equal weight — Check out & play,
 * Check out, Edit, Unlink — with nothing saying which one was meant. It
 * carries one primary, one quiet, and an overflow now. The primary and
 * the chip are both derived from the same `custodyOf` record and nothing
 * else decides either.
 *
 * The full save path left the row for the overflow menu: it is a debug
 * fact, not a daily one.
 */
export function WorldRow({
  link,
  world,
  me,
  art,
  configured,
  launchOnCheckout,
  /** Offline: custody cannot be confirmed, so nothing that would take a
   * world is offered. The hold you already have still stands. */
  offline,
  extraMeta,
}: {
  link: Link;
  world: SyncWorld | undefined;
  me: string | undefined;
  art: Record<string, Artwork>;
  configured: boolean;
  /** The setting, so the button can promise only what will happen. */
  launchOnCheckout: boolean;
  offline?: boolean;
  extraMeta?: string;
}) {
  const refresh = useRefreshState();
  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState<null | "unlink" | "takeover">(null);
  const custody: Custody = custodyOf(link, world, me, configured);
  const state = custody.state;
  const title = link.gameTitle || world?.world.name || "";
  const line = custodyLine(custody, link, me);
  // "& play" only when both halves are true: the setting is on, and this
  // world has something to start. A world linked by hand from a folder
  // has no app id, so the button goes back to promising the save alone.
  const willPlay = launchOnCheckout && launchable(link);
  const meta = [...headMeta({ world }), ...(extraMeta ? [extraMeta] : [])];

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

  /**
   * Checking out is two halves of one intention: fetch the save, then
   * play. The companion does them in that order and reports both, because
   * a save on disk with a game that would not start is a real outcome —
   * the custody half succeeded and the player needs to know the other
   * half did not, without being told the whole thing failed.
   */
  const checkout = async (takeover: boolean, play = true) => {
    try {
      const out = await api.checkout(link.worldId, takeover, play);
      if (out.launchError) {
        toast.warning(`checked out, but the game did not start: ${out.launchError}`);
      } else if (out.launched) {
        toast.success("checked out — the save is on this machine, and the game is starting");
      } else {
        toast.success("checked out — the save is on this machine");
      }
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      refresh();
    }
  };

  const items: MenuItem[] = [];
  if (state === "mine") {
    items.push({ label: "Renew hold", onSelect: () => run(() => api.renew(link.worldId), "hold renewed") });
    if (world?.world.checkpoints) {
      items.push({
        label: "Checkpoint now",
        onSelect: () => run(() => api.checkpoint(link.worldId), "checkpoint pushed"),
      });
    }
  }
  if (nativeFolders()) {
    items.push({
      label: "Open save folder",
      onSelect: async () => {
        if (!(await openPath(link.dir))) toast.error("this build cannot open a folder for you");
      },
    });
  }
  items.push({
    label: "Copy save path",
    onSelect: async () => {
      try {
        await navigator.clipboard.writeText(link.dir);
        toast.success("save path copied");
      } catch {
        toast.error("the clipboard is not available here");
      }
    },
  });
  items.push({ label: "Rename or move…", onSelect: () => setEditing(true) });
  items.push({ label: "Unlink", danger: true, separated: true, onSelect: () => setConfirm("unlink") });

  return (
    <div className="flex items-center gap-4 px-[18px] py-[15px] transition-colors hover:bg-well">
      <div className={cn("flex-none", state === "held" && "opacity-75")}>
        <CoverArt art={art} game={{ appId: link.appId, name: title }} variant="thumb" />
      </div>

      <div className="flex min-w-0 flex-1 flex-col gap-[5px]">
        <div className="flex flex-wrap items-baseline gap-2.5">
          <span className="text-[17px] font-bold text-parchment">
            {world ? world.world.name : `world #${link.worldId}`}
          </span>
          {link.gameTitle ? <span className="text-[12px] text-rune">{link.gameTitle}</span> : null}
        </div>
        <div className="flex flex-wrap items-center gap-2.5 text-[12.5px] text-mist">
          <CustodyChip custody={custody} />
          {line ? <span>{line}</span> : null}
          <HoldCountdown custody={custody} />
        </div>
      </div>

      {meta.length ? (
        <div className="text-right font-mono text-[11px] leading-[1.5] text-mist">
          {meta.map((m) => (
            <div key={m}>{m}</div>
          ))}
        </div>
      ) : null}

      <div className="ml-2 flex items-center gap-2">
        {state === "free" && !offline ? (
          <>
            <Button variant="primary" onClick={() => checkout(false)}>
              <Play className="h-[11px] w-[11px] fill-current" aria-hidden />
              {willPlay ? "Check out & play" : "Check out & host"}
            </Button>
            {/* The save alone, no launch — for taking custody without
                starting anything, regardless of the setting. */}
            {willPlay ? (
              <Button onClick={() => checkout(false, false)}>Check out only</Button>
            ) : null}
          </>
        ) : null}

        {state === "mine" ? (
          <>
            {launchable(link) ? (
              <Button
                variant="primary"
                onClick={() => run(() => api.launch(link.worldId), "starting the game")}
              >
                <Play className="h-[11px] w-[11px] fill-current" aria-hidden />
                Play
              </Button>
            ) : null}
            <Button
              variant={launchable(link) ? "quiet" : "primary"}
              disabled={offline}
              title={offline ? "the vault is unreachable — the hold stands until it answers" : undefined}
              onClick={() => run(() => api.checkin(link.worldId), "checked in — the world is free")}
            >
              Check in
            </Button>
          </>
        ) : null}

        {state === "expired" && !offline ? (
          <Button variant="primary" onClick={() => setConfirm("takeover")}>
            Take over expired hold
          </Button>
        ) : null}

        {(state === "held" || state === "expired") && !custody.claimedBy && !offline ? (
          <Button
            onClick={() =>
              run(
                () => api.claim(link.worldId),
                "you're next — the world downloads automatically when it frees up",
              )
            }
          >
            Ask for it back
          </Button>
        ) : null}

        <OverflowMenu header={link.dir} items={items} />
      </div>

      {editing ? <EditWorldDialog link={link} world={world} onClose={() => setEditing(false)} /> : null}
      <ConfirmDialog
        open={confirm === "unlink"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Unlink this world from its folder?"
        body="Nothing is deleted."
        confirmLabel="Unlink"
        danger
        onConfirm={() => {
          setConfirm(null);
          run(() => api.unlink(link.worldId), "unlinked");
        }}
      />
      <ConfirmDialog
        open={confirm === "takeover"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Take over the expired hold?"
        body="The old holder's late check-in is kept and flagged, not lost."
        confirmLabel="Take over"
        onConfirm={() => {
          setConfirm(null);
          checkout(true);
        }}
      />
    </div>
  );
}
