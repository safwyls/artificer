import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import type { Artwork, World, WorldStatus } from "./types";

// Cover art for the world shelf. Keyed exactly as the service's artwork
// map and the companion's shelf are, so one game has one identity across
// all three.

export interface ArtQuery {
  appId: string;
  name: string;
}

export const gameKey = (g: ArtQuery) =>
  g.appId ? `app:${g.appId}` : `name:${String(g.name || "").toLowerCase().trim()}`;

/**
 * appIdOf digs the Steam app id out of the free-form metadata the
 * companion reported. The server stores that blob without interpreting it,
 * so this is the one place that looks inside — defensively, since anything
 * at all may be in there (and reliquary stays game-blind: an app id is an
 * identity for a cover lookup, never a branch on which game this is).
 */
export function appIdOf(w: Pick<World, "gameMeta">): string {
  try {
    const meta = JSON.parse(w.gameMeta || "{}");
    return meta && meta.appId ? String(meta.appId) : "";
  } catch {
    return "";
  }
}

export const artQueryFor = (w: World): ArtQuery => ({
  appId: appIdOf(w),
  name: w.gameTitle || w.name || "",
});

/**
 * The set of games on screen, as a stable string. Covers are fetched when
 * this changes and never on the refresh timer — the same discipline the
 * companion needed. Asking on every poll would be a lookup every twenty
 * seconds for a list that changes about once a week.
 */
export const artSignature = (worlds: WorldStatus[]) =>
  JSON.stringify([...worlds.map((st) => gameKey(artQueryFor(st.world)))].sort());

/**
 * What is known so far, across every set of worlds asked about. Kept
 * outside react-query's cache so a *different* set of worlds (the detail
 * page's one world, say) still renders the cover the list already fetched
 * rather than a placeholder while its own lookup runs.
 */
const known: Record<string, Artwork> = {};

/** Test seam: the module cache would otherwise leak between cases. */
export function resetArtCache() {
  for (const k of Object.keys(known)) delete known[k];
}

/** The cover for one world, by app id if the companion reported one and by
 * name otherwise — with the name as a second chance, since a world may be
 * keyed by app id here and by name in a lookup that came back earlier. */
export function artFor(w: World): Artwork {
  const q = artQueryFor(w);
  return (
    known[gameKey(q)] ??
    (q.name ? known[`name:${q.name.toLowerCase().trim()}`] : undefined) ??
    {}
  );
}

/**
 * Fetch the covers for a set of worlds, once per distinct set. Failures are
 * silent: covers are decoration, and a shelf of names is a working shelf.
 */
export function useArtwork(worlds: WorldStatus[]) {
  const sig = artSignature(worlds);
  const query = useQuery({
    queryKey: ["artwork", sig],
    enabled: worlds.length > 0,
    // The answer is good until the *set* changes, and the set is the key —
    // so this never refetches on a poll, a refocus, or a custody event.
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
    queryFn: async () => {
      try {
        const out = await api.artwork(worlds.map((st) => artQueryFor(st.world)));
        Object.assign(known, out.art ?? {});
      } catch {
        // covers are decoration; a failure changes nothing
      }
      return sig;
    },
  });
  // The value is the module cache; the query exists for its side effect and
  // its once-per-set discipline. Returning its data makes the render depend
  // on the fetch settling, so covers appear when they arrive.
  return { ready: query.data === sig, artFor };
}
