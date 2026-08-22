import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./lib/auth";
import { AppShell } from "./components/AppShell";
import { Toaster } from "./components/ui/toaster";
import { Login } from "./pages/Login";
import { Worlds } from "./pages/Worlds";
import { WorldDetail } from "./pages/WorldDetail";
import { Companion } from "./pages/Companion";
import { AdminUsers } from "./pages/AdminUsers";
import { AdminArtwork } from "./pages/AdminArtwork";
import { AdminCatalogue } from "./pages/AdminCatalogue";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { username, loading } = useAuth();
  // Nothing while the Cloudflare Access attempt is in flight: a login form
  // that flashes and vanishes is worse than a moment of blank.
  if (loading) return <p className="p-8 text-mist">Opening the vault…</p>;
  if (!username) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

/** The admin APIs answer non-admins with an error; landing one on the page
 * would just render that error, so bounce them home instead. */
function RequireAdmin({ children }: { children: React.ReactNode }) {
  const { isAdmin } = useAuth();
  if (!isAdmin) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export function App() {
  return (
    <>
      <Toaster />
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <RequireAuth>
              <AppShell />
            </RequireAuth>
          }
        >
          <Route path="/" element={<Worlds />} />
          <Route path="/worlds/:worldID" element={<WorldDetail />} />
          <Route path="/companion" element={<Companion />} />
          <Route
            path="/admin/users"
            element={
              <RequireAdmin>
                <AdminUsers />
              </RequireAdmin>
            }
          />
          <Route
            path="/admin/artwork"
            element={
              <RequireAdmin>
                <AdminArtwork />
              </RequireAdmin>
            }
          />
          <Route
            path="/admin/catalogue"
            element={
              <RequireAdmin>
                <AdminCatalogue />
              </RequireAdmin>
            }
          />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
}
