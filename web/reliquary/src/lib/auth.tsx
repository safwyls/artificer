import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { ApiError, api, setUnauthorizedHandler } from "./api";
import type { Me } from "./types";

/** What the boot-time Cloudflare Access attempt had to say. Someone
 * already signed in at Access shouldn't sign in twice — the assertion
 * riding the request is the credential. When that doesn't land, the login
 * page SAYS WHY: a silent fall-through to the password box is
 * indistinguishable from SSO being broken, and the two usual causes have
 * different fixes. */
export type SsoHint =
  | { kind: "none" }
  /** 404: reliquary has no Access configuration, whatever is in front of it. */
  | { kind: "unconfigured" }
  /** 401: the request arrived without an assertion. */
  | { kind: "no-assertion" }
  /** Anything else, including the 403 for an account Access knows and
   * reliquary refuses. */
  | { kind: "error"; message: string };

interface AuthState {
  me: Me | null;
  username: string | null;
  isAdmin: boolean;
  /** World custody: the grant that lets an account check a world out, check
   * it back in, and hold a companion token. Admins have it implicitly. The
   * server still enforces every one of these. */
  canSync: boolean;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  ssoHint: SsoHint;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [ssoHint, setSsoHint] = useState<SsoHint>({ kind: "none" });

  useEffect(() => {
    let cancelled = false;
    // Boot order, verbatim from the page this replaces: an existing
    // session first, then — only if there isn't one — Cloudflare Access.
    const boot = async (): Promise<Me | null> => {
      try {
        return await api.me();
      } catch {
        // No session. Try the front door.
      }
      try {
        await api.loginCloudflare();
        return await api.me();
      } catch (err) {
        if (!cancelled) setSsoHint(hintFor(err));
        return null;
      }
    };
    boot()
      .then((who) => {
        if (!cancelled) setMe(who);
      })
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

  const login = async (username: string, password: string) => {
    await api.login(username, password);
    // Re-read rather than trusting the login response: /me is the single
    // source for role and permissions.
    setMe(await api.me());
  };

  const logout = async () => {
    const ssoLogout = me?.ssoLogoutURL;
    await api.logout().catch(() => {});
    setMe(null);
    // A session that began at Access has to end there too — clearing only
    // this cookie would leave the next person at the browser holding the
    // previous identity.
    if (ssoLogout) window.location.href = ssoLogout;
  };

  const value: AuthState = {
    me,
    username: me?.username ?? null,
    isAdmin: Boolean(me?.isAdmin),
    canSync: Boolean(me?.isAdmin) || Boolean(me?.permissions?.includes("savesync")),
    loading,
    login,
    logout,
    ssoHint,
  };
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function hintFor(err: unknown): SsoHint {
  if (!(err instanceof ApiError)) return { kind: "none" };
  if (err.status === 404) return { kind: "unconfigured" };
  if (err.status === 401) return { kind: "no-assertion" };
  return { kind: "error", message: err.message };
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
