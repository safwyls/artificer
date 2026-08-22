import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { PanelBoundary } from "./Panel";

function Explodes(): JSX.Element {
  throw new Error("cannot read properties of null");
}

afterEach(() => vi.restoreAllMocks());

// Regression: one section throwing used to take the whole page with it —
// the shelf, the scan trail and the version line all sat downstream of a
// single unguarded read, so one null turned into three bugs that looked
// unrelated.
describe("PanelBoundary", () => {
  it("contains a failure to its own panel, and names it", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <>
        <PanelBoundary name="shelf">
          <Explodes />
        </PanelBoundary>
        <PanelBoundary name="worlds">
          <p>the worlds still render</p>
        </PanelBoundary>
      </>,
    );
    expect(screen.getByText(/The shelf panel failed: cannot read properties of null/)).toBeInTheDocument();
    expect(screen.getByText("the worlds still render")).toBeInTheDocument();
  });

  it("tells the player the rest of the page still works", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PanelBoundary name="shelf">
        <Explodes />
      </PanelBoundary>,
    );
    expect(screen.getByText(/The rest of this page still works/)).toBeInTheDocument();
  });

  it("stays out of the way when nothing is wrong", () => {
    render(
      <PanelBoundary name="shelf">
        <p>tiles</p>
      </PanelBoundary>,
    );
    expect(screen.getByText("tiles")).toBeInTheDocument();
  });
});
