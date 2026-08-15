import { describe, expect, it } from "vitest";
import type { Feature } from "./api";
import { FEATURE_ROUTES, featureBlurb, featureLabel, gameProfile } from "./games";
import { makeServer } from "../test/utils";

describe("gameProfile", () => {
  it("returns the Enshrouded profile by id", () => {
    expect(gameProfile("enshrouded").name).toBe("Enshrouded");
  });

  it("falls back to Enshrouded for an empty id", () => {
    expect(gameProfile("")).toBe(gameProfile("enshrouded"));
    expect(gameProfile(undefined)).toBe(gameProfile("enshrouded"));
  });

  it("falls back for a game this build doesn't know, so nav still works", () => {
    // The backend is the authority on which views exist; an unknown game
    // navigates correctly, just with borrowed words.
    expect(gameProfile("ark")).toBe(gameProfile("enshrouded"));
  });
});

describe("featureLabel", () => {
  it("uses the game's own vocabulary", () => {
    const server = makeServer({ game: "enshrouded" });
    expect(featureLabel(server, "pals" as Feature)).toBe("Flameborn");
    expect(featureLabel(server, "saves" as Feature)).toBe("World saves");
  });

  it("degrades to the raw key for a feature the profile has no word for", () => {
    expect(featureLabel(makeServer(), "brandnew" as Feature)).toBe("brandnew");
  });

  it("works with no server at all", () => {
    expect(featureLabel(undefined, "map" as Feature)).toBe("Map");
  });
});

describe("featureBlurb", () => {
  it("describes a view from the player's side", () => {
    expect(featureBlurb(makeServer(), "map" as Feature)).toMatch(/where players are/i);
  });

  it("is empty for an unknown feature", () => {
    expect(featureBlurb(makeServer(), "brandnew" as Feature)).toBe("");
  });
});

describe("FEATURE_ROUTES", () => {
  it("keeps the pals view at its historical /players segment", () => {
    // The segment is part of the URL contract; changing it breaks bookmarks.
    expect(FEATURE_ROUTES.pals).toBe("players");
  });

  it("covers every labelled feature", () => {
    for (const key of Object.keys(gameProfile("enshrouded").labels) as Feature[]) {
      expect(FEATURE_ROUTES[key], key).toBeTruthy();
    }
  });
});
