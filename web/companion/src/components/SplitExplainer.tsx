import { useEffect, useState } from "react";
import { api, errorText } from "../lib/api";
import type { DiscoveredGame, SplitInfo } from "../lib/types";

/**
 * The two halves of a save folder, shown before anything is created.
 *
 * Joining an existing world is the case this exists for. A world's folder
 * is often an opaque id an Unreal game generated once
 * (K2hAc0p_LH74aymwOemkgg): everyone playing that world shares it, nobody
 * can retype it, and the game will not create it until it has saved
 * there. So the joining player supplies only the half they can know — the
 * folder their game keeps saves in — and the companion makes the rest
 * underneath.
 *
 * Creating a world is the mirror: the folder being linked is split, the
 * player is shown where the line falls, and the leaf is recorded so
 * everyone who joins later gets it made for them.
 */
export function SplitExplainer({
  dir,
  leaf,
  game,
  onSplit,
}: {
  dir: string;
  /** The world's own folder name, when joining a world that has one. */
  leaf: string;
  game: DiscoveredGame;
  /** Reports the split back, because creating a world records its leaf. */
  onSplit: (split: SplitInfo | null) => void;
}) {
  const [joining, setJoining] = useState<{ dir: string; exists: boolean } | null>(null);
  const [creating, setCreating] = useState<SplitInfo | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setError("");
    if (leaf) {
      setCreating(null);
      onSplit(null);
      if (!dir) {
        setJoining(null);
        return;
      }
      // create:false — nothing is made until the player submits.
      api
        .resolveSavePath(dir, leaf, false)
        .then((out) => !cancelled && setJoining(out))
        .catch((err) => {
          if (!cancelled) {
            setJoining(null);
            setError(errorText(err));
          }
        });
      return () => {
        cancelled = true;
      };
    }
    setJoining(null);
    if (!dir) {
      setCreating(null);
      onSplit(null);
      return;
    }
    api
      .splitSavePath(dir, game.appId ?? "", game.name ?? "")
      .then((out) => {
        if (cancelled) return;
        setCreating(out.split ?? null);
        onSplit(out.split ?? null);
      })
      .catch(() => {
        if (cancelled) return;
        setCreating(null);
        onSplit(null);
      });
    return () => {
      cancelled = true;
    };
    // onSplit is the parent's setter; re-running on its identity would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dir, leaf, game.appId, game.name]);

  const box = "border-l-[3px] border-rune bg-rune/10 px-2.5 py-[7px] text-[12.5px]";
  const leafName = "font-mono text-goldhi break-all";
  const rootName = "font-mono text-mist break-all";

  if (error) return <div className={box}>{error}</div>;

  if (leaf) {
    if (!joining) return null;
    return (
      <div className={box}>
        This world lives in a folder named <span className={leafName}>{leaf}</span>. Point at the folder your game
        keeps its saves in above, and linking will {joining.exists ? "use" : <b>create</b>}:
        <br />
        <span className={rootName}>{joining.dir}</span>
        {joining.exists ? null : (
          <>
            <br />
            <span className="text-[12px] italic text-mist">
              It does not exist yet — that is expected if you have never played this world.
            </span>
          </>
        )}
      </div>
    );
  }

  if (!creating?.leaf) return null;
  return (
    <div className={box}>
      This world will be recorded as the folder <span className={leafName}>{creating.leaf}</span>, inside{" "}
      <span className={rootName}>{creating.root}</span>
      {creating.why ? ` — ${creating.why}` : ""}. Anyone else joining picks their own save folder and gets that same
      folder name created for them, so they never need to know it.
    </div>
  );
}
