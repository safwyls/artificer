import type { ReactNode } from "react";
import { VaultMark } from "./VaultMark";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";
import type { CompanionState } from "../lib/types";

function Step({
  done,
  n,
  title,
  sub,
  action,
}: {
  done?: boolean;
  n: number;
  title: string;
  sub: string;
  action?: ReactNode;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3.5 border-t border-edge px-5 py-4 first:border-t-0",
        // The one step that is still to do carries the ground of a well,
        // so "where am I" is answered before anything is read.
        !done && "bg-well",
      )}
    >
      <span
        className={cn(
          "flex h-[22px] w-[22px] flex-none items-center justify-center rounded-full border font-mono text-[11px]",
          done ? "border-ok text-ok" : "border-gold text-gold",
        )}
      >
        {done ? "✓" : n}
      </span>
      <div className="flex-1">
        <div className="text-[14.5px] text-parchment">{title}</div>
        <div className="text-[12.5px] text-mist">{sub}</div>
      </div>
      {action}
    </div>
  );
}

/**
 * The machine is connected to a vault but holds no worlds — the state the
 * redesign keys on "no linked worlds" rather than on a stored first-run
 * flag, so it comes back on its own if every link is removed.
 *
 * It says what is already true before it asks for anything, because the
 * two facts a player needs to trust the next step — that the vault
 * answered, and that their games were found — are both already known by
 * the time this screen renders.
 */
export function NoWorlds({ state, onOpenGames }: { state: CompanionState; onOpenGames: () => void }) {
  const games = (state.discovered?.games ?? []).filter((g) => !g.hidden);
  const withFolder = games.filter((g) => (g.saveDirs ?? []).length).length;
  const libraries = state.discovered?.libraries?.length ?? 0;
  const worlds = state.sync?.worlds?.length ?? 0;

  return (
    <div className="flex justify-center px-7 py-[60px]">
      <div className="flex w-full max-w-[620px] flex-col gap-[22px]">
        <div className="flex flex-col items-center gap-2 text-center">
          <VaultMark className="h-[34px] w-[34px] text-gold" />
          <h1 className="text-[21px] tracking-[0.05em] text-gold">No worlds on this machine yet</h1>
          <p className="max-w-[46ch] text-[14px] text-mist">
            A world is one save folder the vault holds for your group. Link a game&apos;s save folder
            and the vault starts keeping its history.
          </p>
        </div>

        <div className="overflow-hidden rounded-panel border border-edge bg-panel">
          <Step
            done
            n={1}
            title={`Signed in as ${state.sync?.username ?? "this machine"}`}
            sub={state.config?.serverUrl || "connected to your vault"}
          />
          <Step
            done
            n={2}
            title={`Found ${games.length} installed game${games.length === 1 ? "" : "s"}`}
            sub={
              (libraries ? `Across ${libraries} librar${libraries === 1 ? "y" : "ies"}. ` : "") +
              `A save folder is already known for ${withFolder} of them.`
            }
            action={
              <Button type="button" onClick={onOpenGames}>
                Review
              </Button>
            }
          />
          <Step
            n={3}
            title="Link a game to make your first world"
            sub="Pick the game and the companion suggests the save folder. You confirm it."
            action={
              <Button type="button" variant="primary" size="lg" onClick={onOpenGames}>
                Choose a game
              </Button>
            }
          />
        </div>

        <p className="text-center text-[12.5px] text-mist">
          {worlds
            ? `Your group already has ${worlds} world${worlds === 1 ? "" : "s"}. `
            : "Someone in your group already made worlds? "}
          <button
            type="button"
            onClick={onOpenGames}
            className="rounded-[3px] text-goldhi hover:text-gold hover:underline"
          >
            Join one instead
          </button>
        </p>
      </div>
    </div>
  );
}
