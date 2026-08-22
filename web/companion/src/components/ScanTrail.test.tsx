import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ScanTrail } from "./ScanTrail";
import { makeProbe } from "../test/utils";

const found = [makeProbe({ resolved: "C:\\Program Files (x86)\\Steam" })];
const missed = [makeProbe({ source: "default", path: "E:\\Steam", note: "does not exist" })];

const details = () => document.querySelector("details") as HTMLDetailsElement;

describe("ScanTrail", () => {
  it("shows nothing at all before the first scan", () => {
    const { container } = render(<ScanTrail probes={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("counts libraries and paths, agreeing with itself on one", () => {
    render(<ScanTrail probes={found} />);
    expect(screen.getByText(/1 library found, 1 path tried/)).toBeInTheDocument();
  });

  // The trail is the whole answer to "why no games", so it opens itself
  // exactly when there is a question to answer.
  it("starts closed when the scan found something", () => {
    render(<ScanTrail probes={found} />);
    expect(details().open).toBe(false);
  });

  it("starts open when the scan found nothing", () => {
    render(<ScanTrail probes={missed} />);
    expect(details().open).toBe(true);
  });

  // Regression: rewriting the trail's markup on every poll snapped it
  // shut two seconds after anyone expanded it. The open state is the
  // player's and must survive the poll.
  it("keeps the player's choice across polls that change nothing", async () => {
    const { rerender } = render(<ScanTrail probes={found} />);
    await userEvent.click(screen.getByText(/scan trail/));
    expect(details().open).toBe(true);
    // Five seconds later, the same probes arrive as new objects.
    rerender(<ScanTrail probes={[makeProbe({ resolved: "C:\\Program Files (x86)\\Steam" })]} />);
    expect(details().open).toBe(true);
  });

  it("keeps it closed across polls too, when that was the choice", async () => {
    const { rerender } = render(<ScanTrail probes={missed} />);
    await userEvent.click(screen.getByText(/scan trail/));
    expect(details().open).toBe(false);
    rerender(<ScanTrail probes={[makeProbe({ source: "default", path: "E:\\Steam", note: "does not exist" })]} />);
    expect(details().open).toBe(false);
  });

  it("opens itself when a fresh scan comes back empty-handed", () => {
    const { rerender } = render(<ScanTrail probes={found} />);
    expect(details().open).toBe(false);
    rerender(<ScanTrail probes={missed} />);
    expect(details().open).toBe(true);
  });

  it("shows why a miss missed, and what a hit resolved to", () => {
    render(
      <ScanTrail
        probes={[
          makeProbe({ source: "libraryfolders", path: "D:\\SteamLibrary", resolved: "D:\\SteamLibrary\\steamapps" }),
          ...missed,
        ]}
      />,
    );
    expect(screen.getByText("→ D:\\SteamLibrary\\steamapps")).toBeInTheDocument();
    expect(screen.getByText("does not exist")).toBeInTheDocument();
  });
});
