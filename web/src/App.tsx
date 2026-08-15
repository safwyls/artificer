import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./lib/auth";
import { Login } from "./pages/Login";
import { EmptyState } from "./pages/EmptyState";
import { Users } from "./pages/Users";
import { ServerAutomation } from "./pages/ServerAutomation";
import { ServerActivity } from "./pages/ServerActivity";
import { PublicStatus } from "./pages/PublicStatus";
import { FkOverview } from "./pages/flamekeeper/FkOverview";
import { FkFlameborn } from "./pages/flamekeeper/FkFlameborn";
import { FkSaves } from "./pages/flamekeeper/FkSaves";
import { FkLogs } from "./pages/flamekeeper/FkLogs";
import { FkConfig } from "./pages/flamekeeper/FkConfig";
import { AppShell } from "./components/AppShell";
import { FeatureGate } from "./components/FeatureGate";
import { Toaster } from "./components/ui/sonner";
import { TooltipProvider } from "./components/ui/tooltip";

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
          <Route path="/servers/:serverID" element={<FkOverview />} />
          <Route path="/servers/:serverID/settings" element={<FkConfig />} />
          <Route path="/servers/:serverID/automation" element={<ServerAutomation />} />
          <Route path="/servers/:serverID/activity" element={<ServerActivity />} />
          <Route
            path="/servers/:serverID/players"
            element={
              <FeatureGate feature="pals">
                <FkFlameborn />
              </FeatureGate>
            }
          />
          <Route
            path="/servers/:serverID/saves"
            element={
              <FeatureGate feature="saves">
                <FkSaves />
              </FeatureGate>
            }
          />
          <Route
            path="/servers/:serverID/logs"
            element={
              <FeatureGate feature="logs">
                <FkLogs />
              </FeatureGate>
            }
          />
        </Route>
      </Routes>
    </TooltipProvider>
  );
}
