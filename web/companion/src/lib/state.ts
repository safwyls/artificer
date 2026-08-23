import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorText } from "./api";
import { LIVE_POLL_MS } from "./events";
import { gameKey, type Artwork, type CompanionState, type DiscoveredGame } from "./types";

/** How often the open page re-reads local state. The companion's own
 * handler treats a read as "someone is looking" and refreshes from the
 * service in the background, so this is also what keeps custody live. */
export const POLL_MS = 5_000;

/**
 * @param live — the SSE stream is carrying changes, so the poll drops to
 * a heartbeat. It does not stop: a stream can be dropped by anything in
 * between without either end noticing, and custody is the wrong thing to
 * be quietly wrong about.
 */
export function useCompanionState(live = false) {
  return useQuery<CompanionState>({
    queryKey: ["state"],
    queryFn: api.state,
    refetchInterval: live ? LIVE_POLL_MS : POLL_MS,
    // The page is a local process's own view; refetching on focus on top
    // of a 5 s poll buys nothing.
    refetchOnWindowFocus: false,
  });
}

/** Re-read local state now — after anything that changed it. */
export function useRefreshState() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ["state"] });
}

/** The set of games on the shelf, as a stable string. */
export const gameSignature = (games: DiscoveredGame[]) =>
  JSON.stringify(games.map((g) => gameKey(g)).sort());

/**
 * Covers, fetched whenever the discovered game set changes — **not** once
 * at page load, which was the bug that made a perfectly good IGDB
 * credential look dead: discovery is a filesystem walk that finishes
 * after the first render, so the one boot-time call always found an empty
 * shelf, asked for nothing, and never ran again. The service's own
 * counter read "0 asked" while its credentials tested fine.
 *
 * Keying the query by the signature is what makes that structural: a new
 * set is a new key, and the same set is never asked twice.
 */
export function useArtwork(games: DiscoveredGame[]) {
  const sig = gameSignature(games);
  const query = useQuery({
    queryKey: ["artwork", sig],
    enabled: games.length > 0,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
    queryFn: async () => {
      try {
        return await api.artwork();
      } catch {
        // Covers are decoration; a failure changes nothing.
        return { art: {} as Record<string, Artwork> };
      }
    },
  });
  return {
    art: query.data?.art ?? {},
    /** The service was asked and had nothing — worth saying, because it
     * points at its Cover art panel rather than at this machine. */
    empty: Boolean(query.data?.asked) && !Object.keys(query.data?.art ?? {}).length,
    error: query.data?.error,
  };
}

/**
 * Save-location hints, on the same trigger as covers: ask when the game
 * set changes, never on a timer. The answer changes which folders the
 * link form offers, so local state is re-read once it lands.
 */
export function useSaveHints(games: DiscoveredGame[], configured: boolean) {
  const sig = gameSignature(games);
  const refresh = useRefreshState();
  const query = useQuery({
    queryKey: ["savehints", sig],
    enabled: games.length > 0 && configured,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
    queryFn: async () => {
      try {
        const out = await api.saveHints();
        // The candidates the companion offers changed underneath us.
        refresh();
        return out;
      } catch {
        // Hints are an improvement, not a dependency.
        return {};
      }
    },
  });
  return query.data ?? {};
}

/**
 * Every linked world's history, read from the vault on demand.
 *
 * Not on the custody poll: it is one request per linked world, neither
 * tab is needed to sync a save, and both are visited rarely. `enabled` is
 * what keeps it that way — the query only runs while one of the two tabs
 * that reads it is on screen.
 *
 * Errors are surfaced rather than swallowed. Cover art degrades to
 * nothing because it is decoration; a history that quietly shows an empty
 * list is telling you nothing happened, which is the one thing these
 * views must never get wrong.
 */
export function useHistory(enabled: boolean) {
  const query = useQuery({
    queryKey: ["history"],
    enabled,
    // The companion holds a short cache of its own; this one stops a tab
    // switch from re-asking, and `refetch` bypasses both.
    staleTime: 15_000,
    retry: false,
    queryFn: () => api.history(),
  });
  return {
    history: query.data,
    loading: query.isLoading,
    error: query.isError ? errorText(query.error) : undefined,
    refreshing: query.isFetching,
    refresh: () => query.refetch(),
  };
}

/**
 * A text field seeded from the poll but owned by the player: once it has
 * focus, or has been edited, the background refresh stops overwriting it.
 * Without this, typing a service URL loses a character every five seconds.
 */
export function useSeededField(serverValue: string) {
  const [value, setValue] = useState(serverValue);
  const dirty = useRef(false);
  const focused = useRef(false);
  useEffect(() => {
    if (!dirty.current && !focused.current) setValue(serverValue);
  }, [serverValue]);
  return {
    value,
    /** Spread onto the input. */
    props: {
      value,
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => {
        dirty.current = true;
        setValue(e.target.value);
      },
      onFocus: () => {
        focused.current = true;
      },
      onBlur: () => {
        focused.current = false;
      },
    },
    /** After a successful save: let the server's copy lead again. */
    settle: (next?: string) => {
      dirty.current = false;
      focused.current = false;
      if (next !== undefined) setValue(next);
    },
  };
}
