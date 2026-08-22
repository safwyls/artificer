import { useMemo, useRef, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { api, errorText } from "../lib/api";
import { useRefreshState } from "../lib/state";
import { gameKey, type Artwork, type CompanionState, type DiscoveredGame, type SplitInfo } from "../lib/types";
import { CoverArt } from "./CoverArt";
import { FolderBrowser } from "./FolderBrowser";
import { SplitExplainer } from "./SplitExplainer";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";

/** The blank game the by-hand path links: a folder discovery never found. */
export const byHandGame = (): DiscoveredGame => ({
  name: "",
  appId: "",
  installDir: "",
  saveDirs: [],
  byHand: true,
});

function FieldLabel({ children }: { children: React.ReactNode }) {
  return <div className="mb-1 text-[11px] uppercase tracking-[0.1em] text-mist">{children}</div>;
}

/**
 * Linking happens in a dialog rather than inline in the grid: a form
 * wedged between tiles reflows the shelf around it, and on a wide window
 * it lands far from the tile that opened it.
 *
 * Everything the player types lives in this component's own state, so the
 * five-second poll underneath can rebuild the shelf as often as it likes
 * without ever touching a half-filled form.
 */
export function LinkGameDialog({
  game,
  state,
  art,
  onClose,
}: {
  game: DiscoveredGame;
  state: CompanionState;
  art: Record<string, Artwork>;
  onClose: () => void;
}) {
  const refresh = useRefreshState();
  const candidates = game.saveDirs ?? [];
  const [dir, setDir] = useState(candidates[0]?.path ?? "");
  const [worldId, setWorldId] = useState(0);
  const [name, setName] = useState(game.byHand ? "" : game.name);
  const [seed, setSeed] = useState(true);
  const [browsing, setBrowsing] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const split = useRef<SplitInfo | null>(null);
  const dirRef = useRef<HTMLInputElement | null>(null);

  // Only worlds this machine has not already linked: linking a world
  // twice on one machine is two folders claiming one save.
  const worlds = useMemo(
    () =>
      (state.sync?.worlds ?? []).filter(
        (w) => !(state.links ?? []).some((l) => l.worldId === w.world.id),
      ),
    [state.sync?.worlds, state.links],
  );
  const chosen = worlds.find((w) => w.world.id === worldId);
  /** The world's own folder name, when joining a world that records one. */
  const leaf = chosen?.world.savePath ?? "";

  /**
   * linkError reports inside the dialog and keeps it open. Anything that
   * stops a link is the player's next action, so it has to be where they
   * are looking — and the form has to survive to be corrected. Submitting
   * used to close unconditionally and report failures to a status line in
   * a panel behind this box, so a refused link looked exactly like a
   * successful one that did nothing.
   */
  const fail = (msg: string, focus?: HTMLInputElement | null) => {
    setError(msg);
    focus?.focus();
    focus?.scrollIntoView?.({ block: "nearest" });
    return false;
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const gameTitle = (game.name || name).trim();
    const path = dir.trim();
    // Caught here rather than at the service: the answer is a folder the
    // player has to supply, and the round trip only delays the ask.
    if (!path) {
      return fail(
        "This needs the game's save folder before it can link. Paste the folder that holds the save files.",
        dirRef.current,
      );
    }
    if (!worldId && !name.trim()) return fail("A new world needs a name.");

    setBusy(true);
    try {
      let linkDir = path;
      let savePath = "";
      if (worldId && leaf) {
        // Joining a world that knows its own folder: what the player gave
        // is their save root, and the world's folder is created beneath
        // it. This is the whole point — the folder name is usually an
        // opaque id they have no way to type.
        linkDir = (await api.resolveSavePath(path, leaf, true)).dir;
      } else if (!worldId) {
        // Creating: record the folder this world lives in, so the next
        // player gets it made for them.
        savePath = split.current?.leaf ?? "";
      }
      const meta = JSON.stringify({ appId: game.appId ?? "", installDir: game.installDir ?? "" });
      if (worldId) {
        await api.addLink({ worldId, gameTitle, dir: linkDir, meta, appId: game.appId ?? "" });
        toast.success("linked");
      } else {
        await api.createWorld({
          name: name.trim(),
          gameTitle,
          dir: linkDir,
          meta,
          appId: game.appId ?? "",
          savePath,
          seed,
        });
        toast.success(seed ? "world created and seeded with the current save" : "world created and linked");
      }
      onClose();
      refresh();
    } catch (err) {
      fail(errorText(err), dirRef.current);
    } finally {
      setBusy(false);
    }
  };

  const hide = async (hidden: boolean) => {
    try {
      await api.hide(game.key || gameKey(game), hidden);
    } catch (err) {
      toast.error(errorText(err));
    }
    onClose();
    refresh();
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-[560px]">
        <div className="flex items-center gap-3">
          {game.byHand ? null : <CoverArt art={art} game={game} variant="thumb" />}
          <DialogTitle className="text-[17px] font-bold normal-case tracking-normal text-goldhi">
            Link {game.name || "a folder"}
          </DialogTitle>
        </div>

        <form onSubmit={submit} className="mt-4 flex flex-col gap-2.5">
          {candidates.length ? (
            <div>
              <FieldLabel>Save folder found on this machine</FieldLabel>
              <select
                aria-label="Save folder found on this machine"
                className="w-full rounded border border-edge bg-ink px-2.5 py-1.5 font-serif text-[13px] text-parchment"
                value={candidates.some((c) => c.path === dir) ? dir : ""}
                onChange={(e) => e.target.value && setDir(e.target.value)}
              >
                {candidates.map((c) => (
                  <option key={c.path} value={c.path}>
                    {c.path} — {c.why}
                  </option>
                ))}
                <option value="">— type a different folder —</option>
              </select>
            </div>
          ) : (
            <p className="border-l-[3px] border-gold bg-gold/10 px-2.5 py-2 text-[13px]">
              <b>No save folder was found for this game.</b> Paste the path below to link it — it is usually under
              %LOCALAPPDATA%, Documents\My Games, or Saved Games. Nothing is linked until you do.
            </p>
          )}

          <div>
            <FieldLabel>
              {leaf ? "Your save folder for this game" : "Save folder"} <span className="text-gold">(required)</span>
            </FieldLabel>
            <Input
              ref={dirRef}
              aria-label={leaf ? "Your save folder for this game" : "Save folder"}
              className="font-mono text-[12px]"
              placeholder="C:\Users\you\AppData\Local\Game\Saved\SaveGames"
              value={dir}
              onChange={(e) => setDir(e.target.value)}
            />
            <Button type="button" size="sm" className="mt-1.5" onClick={() => setBrowsing((v) => !v)}>
              {browsing ? "Hide the browser" : "Browse this computer…"}
            </Button>
          </div>

          {browsing ? (
            <FolderBrowser
              start={dir}
              onUse={(path) => {
                setDir(path);
                setBrowsing(false);
                setError("");
              }}
            />
          ) : null}

          <div>
            <FieldLabel>World on the service</FieldLabel>
            <select
              aria-label="World on the service"
              className="w-full rounded border border-edge bg-ink px-2.5 py-1.5 font-serif text-[13px] text-parchment"
              value={worldId || ""}
              onChange={(e) => setWorldId(Number(e.target.value) || 0)}
            >
              <option value="">— create a new world —</option>
              {worlds.map((w) => (
                <option key={w.world.id} value={w.world.id}>
                  {w.world.name}
                  {w.world.gameTitle ? ` · ${w.world.gameTitle}` : ""}
                </option>
              ))}
            </select>
          </div>

          <SplitExplainer
            dir={dir.trim()}
            leaf={leaf}
            game={game}
            onSplit={(s) => {
              split.current = s;
            }}
          />

          {/* The new-world fields disappear when an existing world is
              chosen — there is nothing to name and nothing to seed. */}
          {worldId ? null : (
            <>
              <div>
                <FieldLabel>New world&apos;s name</FieldLabel>
                <Input
                  aria-label="New world's name"
                  autoFocus={game.byHand}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <label className="flex items-center gap-2 text-[13px]">
                <input type="checkbox" checked={seed} onChange={(e) => setSeed(e.target.checked)} />
                upload this folder&apos;s current save as the world&apos;s first version
              </label>
            </>
          )}

          <div className="mt-1 flex flex-wrap items-center gap-2">
            <Button type="submit" variant="primary" size="lg" disabled={busy}>
              Link
            </Button>
            <Button type="button" onClick={onClose}>
              Cancel
            </Button>
            {/* Offered wherever a shelf entry is open, because the entries
                worth hiding are exactly the ones you only notice by
                clicking them and finding they are not a game. */}
            {game.byHand ? null : (
              <Button type="button" onClick={() => hide(!game.hidden)}>
                {game.hidden ? "Show on shelf" : "Hide from shelf"}
              </Button>
            )}
          </div>

          {error ? (
            <div className="border-l-[3px] border-ember bg-ember/10 px-2.5 py-1.5 text-[13px] text-ember">
              {error}
            </div>
          ) : null}
        </form>
      </DialogContent>
    </Dialog>
  );
}
