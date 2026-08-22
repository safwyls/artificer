import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { useRefreshState } from "../lib/state";
import { gameKey, type Artwork, type DiscoveredGame, type Link, type SyncWorld } from "../lib/types";
import { ConfirmDialog } from "./ConfirmDialog";
import { CoverArt } from "./CoverArt";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";

/**
 * What a *linked* tile opens: what it points at, and the way out. Custody
 * itself lives in "Your worlds" at the top of the page — one world, one
 * place to check it in and out.
 */
export function LinkedGameDialog({
  game,
  link,
  world,
  art,
  onClose,
}: {
  game: DiscoveredGame;
  link: Link;
  world: SyncWorld | undefined;
  art: Record<string, Artwork>;
  onClose: () => void;
}) {
  const refresh = useRefreshState();
  const run = async (fn: () => Promise<unknown>, okMsg?: string) => {
    try {
      await fn();
      if (okMsg) toast.success(okMsg);
    } catch (err) {
      toast.error(errorText(err));
    }
    onClose();
    refresh();
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <div className="flex items-center gap-3">
          <CoverArt art={art} game={game} variant="thumb" />
          <DialogTitle className="text-[17px] font-bold normal-case tracking-normal text-goldhi">
            {game.name}
          </DialogTitle>
        </div>
        <p className="mt-4 text-[14px]">
          Linked to <b>{world ? world.world.name : `world #${link.worldId}`}</b> — check it out and in from{" "}
          <b>Your worlds</b> at the top of this page.
        </p>
        <div className="mt-2 break-all font-mono text-[12px] text-mist">{link.dir}</div>
        <div className="mt-5 flex flex-wrap gap-2">
          <ConfirmDialog
            trigger={<Button>Unlink</Button>}
            title="Unlink this game?"
            body="Nothing is deleted."
            confirmLabel="Unlink"
            onConfirm={() => run(() => api.unlink(link.worldId), "unlinked")}
          />
          <Button
            onClick={() => run(() => api.hide(game.key || gameKey(game), !game.hidden))}
          >
            {game.hidden ? "Show on shelf" : "Hide from shelf"}
          </Button>
          <Button onClick={onClose}>Close</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
