import type { Feature, Server } from "./api";

/**
 * Per-game presentation. The console navigates by feature key and the API
 * says which keys a server offers; this module says what to *call* them.
 * One game today — the structure stays because it is the seam a second
 * game's vocabulary would slot into (see docs/porting-to-another-game.md).
 *
 * Route segments deliberately do NOT live here: they're part of the URL
 * contract and stay the same whatever a game calls a view, so a bookmark
 * survives and a shared link means one thing.
 */
export interface GameProfile {
  id: string;
  name: string;
  /** Nav and settings labels, per feature key. */
  labels: Record<Feature, string>;
  /** What each view exposes, for the settings page. Written from the player's
   * side — what someone would be agreeing to when they ask to be hidden. */
  blurbs: Record<Feature, string>;
}

const ENSHROUDED: GameProfile = {
  id: "enshrouded",
  name: "Enshrouded",
  labels: {
    map: "Map",
    pals: "Flameborn",
    inventory: "Inventory",
    storage: "Storage",
    paldex: "Bestiary",
    achievements: "Achievements",
    guilds: "Parties",
    calculators: "Calculators",
    saves: "World saves",
    logs: "Server log",
  },
  blurbs: {
    map: "Where players are in Embervale",
    pals: "Who is at the fire now, with join and leave history",
    inventory: "What each player is carrying",
    storage: "What's stored at the base",
    paldex: "Creatures encountered",
    achievements: "Quests completed",
    guilds: "Party rosters",
    calculators: "Crafting tools",
    saves: "World save snapshots, downloadable by admins",
    logs: "The server's own log, join and leave lines included",
  },
};

const GAMES: Record<string, GameProfile> = {
  [ENSHROUDED.id]: ENSHROUDED,
};

/**
 * The profile for a game id. Falls back to Enshrouded for an empty or
 * unknown id — the backend is the authority on which views exist, so an
 * unknown game still navigates correctly, just with borrowed words.
 */
export function gameProfile(id: string | undefined): GameProfile {
  return (id && GAMES[id]) || ENSHROUDED;
}

/** The label for one of a server's views. */
export function featureLabel(server: Server | undefined, feature: Feature): string {
  return gameProfile(server?.game).labels[feature] ?? feature;
}

/** The settings-page description for one of a server's views. */
export function featureBlurb(server: Server | undefined, feature: Feature): string {
  return gameProfile(server?.game).blurbs[feature] ?? "";
}

/**
 * Where each view lives under /servers/:id. The segment differs from the
 * feature key for pals — the players page's URL predates the vocabulary.
 */
export const FEATURE_ROUTES: Record<Feature, string> = {
  map: "map",
  pals: "players",
  inventory: "inventory",
  storage: "storage",
  paldex: "paldex",
  achievements: "achievements",
  guilds: "guilds",
  calculators: "calculators",
  saves: "saves",
  logs: "logs",
};
