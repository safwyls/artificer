import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { freshness } from "../lib/format";
import { inShell, shellPlatform } from "../lib/runtime";
import { cn } from "../lib/utils";
import { VaultMark } from "./VaultMark";
import type { CompanionState } from "../lib/types";

/**
 * The strip across the top of the window: who this is, which machine and
 * account it is, and how fresh that is — the whole of the app's identity
 * and its whole sync report, in one line.
 *
 * Under the desktop shell this strip *is* the window's titlebar. The
 * shell asks the OS for a frameless window (`titleBarStyle: "hidden"` in
 * companion-desktop/src/main.ts), so this is what the window is dragged
 * by, and double-clicking it maximises. Only the caption buttons are
 * still the platform's, recoloured to sit in it — a hand-drawn set of
 * them is what costs Windows 11 its snap-layouts menu on hover.
 *
 * The height and the usable width come from the OS, not from a number
 * agreed by hand: `env(titlebar-area-height)` and
 * `env(titlebar-area-width)` are the Window Controls Overlay's own
 * report of where it drew those buttons (see `.app-titlebar` in
 * index.css, which also explains why the strip is one pixel taller than
 * what the OS reports). Both have fallbacks, which is what the browser
 * build uses — there the strip is just the app's header, drawn under a
 * real titlebar that belongs to the browser.
 *
 * There is no second header under this one. Everything that was in one
 * is here, or has moved to the tab row beside the tabs.
 */
export function TitleBar({
  state,
  syncing = false,
}: {
  state?: CompanionState;
  /** A manual "Sync now" in flight. Client-side, and distinct from the
   * daemon's own `busy` — the report has to cover both or it goes quiet
   * during the one sync the player asked for by hand. */
  syncing?: boolean;
}) {
  const shell = inShell();
  const mac = shellPlatform() === "darwin";
  const sync = state?.sync;
  const configured = Boolean(sync?.configured);
  const offline = Boolean(sync?.lastError);

  // Freshness is an age, so it runs on a clock of its own: the poll it
  // describes may answer with an unchanged timestamp, and an age that
  // stops moving reads as a frozen app.
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <div
      className={cn(
        // Height and bottom rule: `.app-titlebar`, in index.css.
        "app-titlebar flex flex-none select-none",
        "border-b border-edge bg-ink",
        // Under the shell the whole strip drags the window (`.app-drag`,
        // in index.css); nothing in it is interactive, so nothing has to
        // opt back out. The browser build's strip drags nothing — it is
        // a header under a titlebar that belongs to the browser.
        shell && "app-drag",
      )}
    >
      <div
        className={cn(
          // The width the OS left us: everything past it is caption
          // buttons drawn over this strip.
          "app-titlebar-inner flex h-full items-center gap-3 pl-3.5 pr-4",
          // macOS draws its traffic lights at the *left* and reports no
          // overlay geometry, so that end is reserved by hand.
          mac && "pl-[80px]",
        )}
      >
        {/* One mark, one weight: the default stroke everywhere, so the
            strip, the empty state and the tray are the same drawing. */}
        <VaultMark className="h-[14px] w-[14px] flex-none text-gold" />
        {/* The app's name is set in the app's own face, not the mono the
            rest of the chrome uses.

            That is a weight decision, not a taste one: the mono stack
            resolves to Consolas here, which ships Regular and Bold and
            nothing between, so `font-semibold` already snapped to Bold —
            600 and 700 rendered the same pixels and there was no heavier
            left to ask for. Georgia has the weight the strip wanted, and
            it is the face the rest of the app speaks in. */}
        <span className="flex-none font-serif text-[13px] font-bold uppercase tracking-[0.1em] text-gold">
          {/* Two products, two names: the desktop shell is the Reliquary
              Companion (its productName, its own release track), and the
              browser-and-tray build is the Artificer Companion. */}
          {shell ? "Reliquary Companion" : "Artificer Companion"}
        </span>

        <span className="truncate text-[12.5px] text-mist" title={machineLine(state)}>
          {machineLine(state)}
        </span>

        {configured ? (
          <span className="ml-auto flex flex-none items-center gap-[7px] font-mono text-[11px] text-mist">
            {syncing || sync?.busy ? (
              <RefreshCw className="h-[9px] w-[9px] animate-spin text-gold" aria-hidden />
            ) : (
              <span
                className={cn("h-[7px] w-[7px] rounded-full", offline ? "bg-ember" : "bg-ok")}
                aria-hidden
              />
            )}
            <span>
              {syncing
                ? "syncing…"
                : sync?.busy
                  ? "transfer in progress…"
                  : offline
                    ? "the vault is unreachable"
                    : freshness(sync?.polledAt)}
            </span>
          </span>
        ) : null}
      </div>
    </div>
  );
}

/**
 * Which machine and account this is — the one thing the strip says that
 * is not the app's own name.
 *
 * The machine is named where it will say its name. "This machine" was a
 * truism on the screen in front of you; it only carries anything by
 * contrast, and the contrast is not on this strip. It stops being a
 * truism the moment the same account syncs from a second PC, which is
 * precisely the case custody has to disambiguate — a hold that is yours
 * but on a different session is a hold on your *other* machine.
 *
 * A host that will not say its name, and an older daemon that does not
 * send one, both fall back to the old wording rather than to a blank.
 */
function machineLine(state: CompanionState | undefined): string {
  const sync = state?.sync;
  const machine = state?.hostname?.trim() || "this machine";
  return sync?.configured
    ? `${machine}, syncing as ${sync.username ?? "…"}`
    : `${machine} — not connected to a vault yet`;
}
