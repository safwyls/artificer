import { useState } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { useRefreshState } from "../lib/state";
import {
  gameKey,
  launchTargetOf,
  type Artwork,
  type DiscoveredGame,
  type Link,
  type SyncWorld,
} from "../lib/types";
import { ConfirmDialog } from "./ConfirmDialog";
import { CoverArt } from "./CoverArt";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

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
  const [target, setTarget] = useState(link.launchTarget ?? "");
  const [saving, setSaving] = useState(false);
  const willOpen = launchTargetOf({ ...link, launchTarget: target });
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

        {/* What starts this game when the world is checked out. Steam
            games answer for themselves; this is for the ones that
            cannot — a non-Steam install, a modded launcher, a shortcut. */}
        <div className="mt-5">
          <Label htmlFor="launch-target">Launch target (optional)</Label>
          <Input
            id="launch-target"
            className="mt-1 font-mono text-[12px]"
            placeholder={link.appId ? `steam://rungameid/${link.appId}` : "D:\\Games\\thegame.exe"}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          />
          <p className="mt-1.5 text-[12px] italic text-mist">
            {willOpen ? (
              <>
                Checking this world out will open <span className="not-italic">{willOpen}</span>.
              </>
            ) : (
              <>
                Nothing here says what starts this game, so checking the world out will fetch the save and leave the
                game to you. A path or a URI the desktop can open — an .exe, a shortcut, another launcher&apos;s
                link. Not a command line; a shortcut carries its arguments already.
              </>
            )}
          </p>
          <Button
            className="mt-2"
            disabled={saving || target === (link.launchTarget ?? "")}
            onClick={async () => {
              setSaving(true);
              try {
                await api.updateLink(link.worldId, { launchTarget: target });
                toast.success(target.trim() ? "launch target saved" : "launch target cleared");
              } catch (err) {
                toast.error(errorText(err));
              } finally {
                setSaving(false);
                refresh();
              }
            }}
          >
            Save launch target
          </Button>
        </div>
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
