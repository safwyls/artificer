import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, ApiError, setUnauthorizedHandler, type Me, type Permission } from "./api";

interface AuthState {
  username: string | null;
  isAdmin: boolean;
  /** Mirrors the server's grants so the UI can hide controls it would
   * reject anyway. The server still enforces every one of them. */
  can: (permission: Permission) => boolean;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Set when Cloudflare Access authenticated someone this console then
   * refused — a disabled account. The login page shows it instead of an
   * unexplained password prompt. */
  ssoError: string | null;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [ssoError, setSsoError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // Boot order: an existing session first, then — only if there isn't
    // one — a silent attempt at Cloudflare Access. Someone arriving
    // through the tunnel is already authenticated there, so asking them
    // to sign in again would be the console forgetting what the front
    // door just did. Everyone else falls through to the password form
    // without ever knowing this was tried.
    const boot = async (): Promise<Me | null> => {
      try {
        return await api.me();
      } catch (err) {
        if (!(err instanceof ApiError && err.status === 401)) throw err;
      }
      try {
        await api.loginCloudflare();
        return await api.me();
      } catch (err) {
        // 404 (SSO not configured) and 401 (not proxied by Access) are
        // the ordinary answers here. A 403 is not: it means Access
        // recognised them and this console refused — a disabled account —
        // which the login page should say out loud rather than present as
        // a blank password form.
        if (err instanceof ApiError && err.status === 403 && !cancelled) {
          setSsoError(err.message);
        }
        return null;
      }
    };
    boot()
      .then((who) => {
        if (!cancelled) setMe(who);
      })
      .catch((err) => console.error(err))
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Any 401 mid-session (expired/revoked cookie) clears auth state, which
  // sends RequireAuth back to /login instead of leaving every page to fail
  // with its own error.
  useEffect(() => {
    setUnauthorizedHandler(() => setMe(null));
    return () => setUnauthorizedHandler(null);
  }, []);

  const login = async (u: string, p: string) => {
    await api.login(u, p);
    // Re-read rather than trusting the login response: /me is the single
    // source for role and permissions.
    setMe(await api.me());
  };

  const logout = async () => {
    const accessLogout = me?.ssoLogoutURL;
    await api.logout();
    setMe(null);
    // An SSO session has two halves. Ending only ours would send them back
    // to a login page Access immediately signs them into again — and on a
    // shared machine, would hand the next person the previous identity.
    if (accessLogout) window.location.href = accessLogout;
  };

  const value: AuthState = {
    username: me?.username ?? null,
    isAdmin: me?.isAdmin ?? false,
    can: (permission) => Boolean(me?.isAdmin) || Boolean(me?.permissions?.includes(permission)),
    loading,
    login,
    logout,
    ssoError,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
