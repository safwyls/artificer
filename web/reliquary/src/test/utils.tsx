import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { render, type RenderOptions } from "@testing-library/react";
import type { AppUser, Holder, Version, World, WorldStatus } from "../lib/types";

/**
 * A query client tuned for tests: no retries (a deliberate 400 would
 * otherwise be retried and the assertion would race the backoff) and no
 * caching between cases.
 */
export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

export function renderWithProviders(
  ui: ReactElement,
  options: RenderOptions & { route?: string } = {},
) {
  const { route = "/", ...rest } = options;
  const queryClient = makeQueryClient();
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter
        initialEntries={[route]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        {children}
      </MemoryRouter>
    </QueryClientProvider>
  );
  return { queryClient, ...render(ui, { wrapper: Wrapper, ...rest }) };
}

/** A fully-populated world; override only what a case cares about. */
export function makeWorld(overrides: Partial<World> = {}): World {
  return {
    id: 1,
    name: "Emberfall",
    gameTitle: "",
    saveHint: "",
    gameMeta: "",
    savePath: "",
    agentUrl: "",
    hasAgentToken: false,
    leaseHours: 6,
    maxBytes: 1 << 30,
    keepVersions: 20,
    checkpoints: false,
    webhookUrl: "",
    createdAt: "2026-08-01T12:00:00Z",
    ...overrides,
  };
}

export function makeVersion(overrides: Partial<Version> = {}): Version {
  return {
    id: 41,
    worldId: 1,
    kind: "checkin",
    conflict: false,
    bytes: 191_000_000,
    sha256: "abc",
    createdAt: "2026-08-20T21:04:00Z",
    ...overrides,
  };
}

export function makeHolder(overrides: Partial<Holder> = {}): Holder {
  return {
    sessionId: 7,
    userId: 2,
    username: "mira",
    serverHeld: false,
    expiresAt: "2026-08-21T23:00:00Z",
    claimable: false,
    ...overrides,
  };
}

export function makeStatus(overrides: Partial<WorldStatus> = {}): WorldStatus {
  return { world: makeWorld(), ...overrides };
}

export function makeUser(overrides: Partial<AppUser> = {}): AppUser {
  return {
    id: 2,
    username: "mira",
    role: "user",
    permissions: ["savesync"],
    disabled: false,
    ...overrides,
  };
}
