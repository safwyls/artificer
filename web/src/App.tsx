import { Suspense, lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./lib/auth";
import { Login } from "./pages/Login";
import { EmptyState } from "./pages/EmptyState";
import { Users } from "./pages/Users";
import { ServerDashboard } from "./pages/ServerDashboard";
import { ServerMap } from "./pages/ServerMap";
import { ServerConfig } from "./pages/ServerConfig";
import { ServerAutomation } from "./pages/ServerAutomation";
import { ServerActivity } from "./pages/ServerActivity";
import { PublicStatus } from "./pages/PublicStatus";
import { AppShell } from "./components/AppShell";
import { Toaster } from "./components/ui/sonner";
import { TooltipProvider } from "./components/ui/tooltip";

// Split out: this route pulls in the pal dex, skill and stat catalogs
// (~190 KB), which nothing else needs. Dashboard and map users never
// download them.
const ServerPlayers = lazy(() =>
  import("./pages/ServerPlayers").then((m) => ({ default: m.ServerPlayers })),
);
const ServerGuilds = lazy(() =>
  import("./pages/ServerGuilds").then((m) => ({ default: m.ServerGuilds })),
);
// Split out: pulls in the breeding table + base-stats catalog, which only
// this route needs.
const ServerCalculators = lazy(() =>
  import("./pages/ServerCalculators").then((m) => ({ default: m.ServerCalculators })),
);
// Split out with the other pal-catalog pages: it walks the full Paldeck
// with icons and names, which only pal-viewer users ever need.
const ServerPaldex = lazy(() => import("./pages/ServerPaldex").then((m) => ({ default: m.ServerPaldex })));

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { username, loading } = useAuth();
  if (loading) return <p className="p-6 text-muted-foreground">Loading...</p>;
  if (!username) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

// The user-management API is admin-only; landing a non-admin here just
// renders "Failed to load users", so bounce them home instead.
function RequireAdmin({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  if (!isAdmin) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export function App() {
  return (
    <TooltipProvider delayDuration={200}>
      <Toaster position="bottom-right" />
      <Routes>
        <Route path="/login" element={<Login />} />
        {/* Public, no session required — the token is the whole gate. */}
        <Route path="/status/:token" element={<PublicStatus />} />
        <Route
          element={
            <RequireAuth>
              <AppShell />
            </RequireAuth>
          }
        >
          <Route path="/" element={<EmptyState />} />
          <Route
            path="/users"
            element={
              <RequireAdmin>
                <Users />
              </RequireAdmin>
            }
          />
          <Route path="/servers/:serverID" element={<ServerDashboard />} />
          <Route path="/servers/:serverID/map" element={<ServerMap />} />
          <Route path="/servers/:serverID/settings" element={<ServerConfig />} />
          <Route path="/servers/:serverID/automation" element={<ServerAutomation />} />
          <Route path="/servers/:serverID/activity" element={<ServerActivity />} />
          <Route
            path="/servers/:serverID/paldex"
            element={
              <Suspense fallback={<p className="p-6 text-muted-foreground">Loading...</p>}>
                <ServerPaldex />
              </Suspense>
            }
          />
          <Route
            path="/servers/:serverID/guilds"
            element={
              <Suspense fallback={<p className="p-6 text-muted-foreground">Loading…</p>}>
                <ServerGuilds />
              </Suspense>
            }
          />
          <Route
            path="/servers/:serverID/players"
            element={
              <Suspense fallback={<p className="p-6 text-muted-foreground">Loading…</p>}>
                <ServerPlayers />
              </Suspense>
            }
          />
          <Route
            path="/servers/:serverID/calculators"
            element={
              <Suspense fallback={<p className="p-6 text-muted-foreground">Loading…</p>}>
                <ServerCalculators />
              </Suspense>
            }
          />
        </Route>
      </Routes>
    </TooltipProvider>
  );
}
