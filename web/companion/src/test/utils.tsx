import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderOptions } from "@testing-library/react";
import { gameKey } from "../lib/types";
import type {
  CompanionState,
  DiscoveredGame,
  Link,
  Probe,
  SyncWorld,
} from "../lib/types";

/** No retries (a deliberate failure would otherwise be retried and the
 * assertion would race the backoff) and no caching between cases. */
export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

export function renderWithProviders(ui: ReactElement, options: RenderOptions = {}) {
  const queryClient = makeQueryClient();
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, ...render(ui, { wrapper: Wrapper, ...options }) };
}

export function makeGame(overrides: Partial<DiscoveredGame> = {}): DiscoveredGame {
  const game: DiscoveredGame = {
    name: "Enshrouded",
    appId: "1203620",
    installDir: "D:\\Steam\\steamapps\\common\\Enshrouded",
    saveDirs: [{ path: "C:\\Users\\you\\AppData\\Roaming\\Enshrouded\\savegame", why: "save-location catalogue" }],
    hidden: false,
    ...overrides,
  };
  // The server fills `key` in when it serves the state, always from the
  // game's own identity — a fixture that pinned it would let two games
  // share a key, which the shelf keys its tiles by.
  return { ...game, key: overrides.key ?? gameKey(game) };
}

export function makeLink(overrides: Partial<Link> = {}): Link {
  return {
    worldId: 1,
    gameTitle: "Enshrouded",
    dir: "C:\\Users\\you\\AppData\\Roaming\\Enshrouded\\savegame",
    appId: "1203620",
    ...overrides,
  };
}

export function makeSyncWorld(overrides: Partial<SyncWorld> = {}): SyncWorld {
  return {
    world: {
      id: 1,
      name: "Embervale",
      gameTitle: "Enshrouded",
      saveHint: "",
      checkpoints: true,
      savePath: "",
      headVersion: 8,
    },
    ...overrides,
  };
}

export function makeProbe(overrides: Partial<Probe> = {}): Probe {
  return { source: "registry", path: "C:\\Program Files (x86)\\Steam", note: "", ...overrides };
}

export function makeState(overrides: Partial<CompanionState> = {}): CompanionState {
  return {
    config: { serverUrl: "https://vault.example.test", tokenSet: true, steamDirs: [], launchOnCheckout: true },
    links: [],
    discovered: { games: [], probes: [] },
    sync: { configured: true, username: "safwyl", worlds: [], busy: false },
    version: "v1.4.2",
    ...overrides,
  };
}
