import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { TitleBar } from "./TitleBar";
import type { CompanionBridge } from "../lib/runtime";
import type { CompanionState } from "../lib/types";

function shell(platform?: string) {
  window.companion = {
    baseUrl: "http://127.0.0.1:41234",
    token: "tok",
    platform,
    pickFolder: vi.fn(async () => null),
    openPath: vi.fn(async () => {}),
  } satisfies Partial<CompanionBridge>;
}

const connected = (over: Record<string, unknown> = {}) =>
  ({
    sync: { configured: true, username: "hazel", polledAt: new Date().toISOString(), ...over },
  }) as unknown as CompanionState;

afterEach(() => {
  delete window.companion;
});

describe("TitleBar", () => {
  // Two products, two names. The desktop shell is the Reliquary
  // Companion — its productName, its own release track — and the
  // browser-and-tray build is the Artificer Companion.
  it("calls itself by the name of the build it is running in", () => {
    render(<TitleBar state={connected()} />);
    expect(screen.getByText("Artificer Companion")).toBeInTheDocument();

    shell("win32");
    const { getByText } = render(<TitleBar state={connected()} />);
    expect(getByText("Reliquary Companion")).toBeInTheDocument();
  });

  it("is what the window is dragged by, but only where there is a window to drag", () => {
    const { container, unmount } = render(<TitleBar state={connected()} />);
    expect(container.firstElementChild).not.toHaveClass("app-drag");
    unmount();

    shell("win32");
    const inShell = render(<TitleBar state={connected()} />);
    expect(inShell.container.firstElementChild).toHaveClass("app-drag");
  });

  // The OS draws the caption buttons over this strip. On Windows and
  // Linux it also *reports* where — `env(titlebar-area-width)` — so the
  // content is bounded by that rather than by a guessed padding, which
  // is what used to leave the buttons sitting across the bottom rule.
  // macOS reports nothing and puts its lights at the left, so that end
  // is reserved by hand.
  it("leaves the OS the end of the strip it draws on", () => {
    shell("win32");
    const { container, unmount } = render(<TitleBar state={connected()} />);
    const win = container.firstElementChild!.firstElementChild!;
    expect(win).toHaveClass("app-titlebar-inner");
    expect(win).not.toHaveClass("pl-[80px]");
    unmount();

    shell("darwin");
    const mac = render(<TitleBar state={connected()} />);
    expect(mac.container.firstElementChild!.firstElementChild!).toHaveClass("pl-[80px]");
  });

  // The machine is named, not gestured at. "This machine" said nothing
  // you could not already see; a name only starts mattering once the
  // same account syncs from a second PC, which is exactly the case
  // custody has to disambiguate.
  it("names the machine and the account, and says plainly when there is neither", () => {
    const { unmount } = render(
      <TitleBar state={{ ...connected(), hostname: "zedsixninety" }} />,
    );
    expect(screen.getByText("zedsixninety, syncing as hazel")).toBeInTheDocument();
    unmount();

    render(
      <TitleBar
        state={
          { sync: { configured: false }, hostname: "zedsixninety" } as unknown as CompanionState
        }
      />,
    );
    expect(screen.getByText("zedsixninety — not connected to a vault yet")).toBeInTheDocument();
  });

  // A locked-down host that will not say its name, and a daemon too old
  // to send one, are the same case: fall back to the old wording rather
  // than to a blank or a dangling comma.
  it("falls back to the old wording when the machine will not say its name", () => {
    const { unmount } = render(<TitleBar state={connected()} />);
    expect(screen.getByText("this machine, syncing as hazel")).toBeInTheDocument();
    unmount();

    // Whitespace is not a name either.
    render(<TitleBar state={{ ...connected(), hostname: "   " }} />);
    expect(screen.getByText("this machine, syncing as hazel")).toBeInTheDocument();
  });

  // The whole sync report, said once and here: everything else that used
  // to say it is gone.
  it("reports sync state, and prefers the reason over the age", () => {
    const { unmount } = render(<TitleBar state={connected()} />);
    // freshness() calls a fresh poll "up to date" and older ones "synced N ago".
    expect(screen.getByText(/up to date|ago/)).toBeInTheDocument();
    unmount();

    const off = render(<TitleBar state={connected({ lastError: "dial tcp: no such host" })} />);
    expect(off.getByText("the vault is unreachable")).toBeInTheDocument();
    off.unmount();

    // A manual "Sync now" is client-side state the daemon's `busy` does
    // not cover; the report would otherwise go quiet during the one sync
    // the player asked for by hand.
    render(<TitleBar state={connected()} syncing />);
    expect(screen.getByText("syncing…")).toBeInTheDocument();
  });

  it("says nothing about sync when there is no vault to sync with", () => {
    render(<TitleBar state={{ sync: { configured: false } } as unknown as CompanionState} />);
    expect(screen.queryByText(/up to date|ago|unreachable|syncing/)).not.toBeInTheDocument();
  });
});
