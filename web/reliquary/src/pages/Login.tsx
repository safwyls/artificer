import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { ShieldPlus } from "lucide-react";
import { api, errorDetail } from "../lib/api";
import { useAuth } from "../lib/auth";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";

/**
 * The password path — and, when Cloudflare Access was supposed to carry
 * someone in and didn't, the explanation. A silent fall-through to this box
 * is indistinguishable from SSO being broken, and the usual causes have
 * different fixes, so each gets its own sentence.
 */
function SsoHint() {
  const { ssoHint } = useAuth();
  if (ssoHint.kind === "none") return null;
  return (
    <div className="mt-3 text-[12px] italic text-mist">
      {ssoHint.kind === "unconfigured" ? (
        <>
          Cloudflare Access sign-in is <b className="not-italic text-parchment">not configured on this server</b>. Even
          with Access in front of the tunnel, reliquary needs CF_ACCESS_TEAM_DOMAIN and CF_ACCESS_AUD set in its
          environment to accept the assertion — until then, sign in with a password.
        </>
      ) : null}
      {ssoHint.kind === "no-assertion" ? (
        <>
          This request didn&apos;t arrive with a Cloudflare Access assertion — you&apos;re reaching reliquary directly
          (LAN address, or a tunnel that strips the header) rather than through Access. Sign in with a password.
        </>
      ) : null}
      {ssoHint.kind === "error" ? <>Cloudflare Access sign-in failed: {ssoHint.message}</> : null}
    </div>
  );
}

export function Login() {
  const { username: signedIn, loading, login } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const version = useQuery({ queryKey: ["version"], queryFn: api.version, staleTime: Infinity });

  if (loading) return null;
  if (signedIn) return <Navigate to="/" replace />;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await login(username, password);
    } catch (err) {
      setError(errorDetail(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-[radial-gradient(ellipse_at_50%_30%,#1a1524_0%,#100d17_65%)] p-6">
      <form
        onSubmit={submit}
        className="flex w-full max-w-[360px] flex-col rounded-panel border border-edge bg-panel px-6 pb-[26px] pt-[30px] sm:px-8"
      >
        <div className="mb-3.5 flex flex-col items-center gap-1.5">
          <ShieldPlus className="h-8 w-8 text-gold" strokeWidth={1.2} aria-hidden />
          <div className="text-[22px] tracking-[0.06em] text-gold">Reliquary</div>
          <div className="text-[13px] text-mist">Sign in to the vault.</div>
        </div>

        <Label htmlFor="login-user" className="mt-1.5">
          Username
        </Label>
        <Input
          id="login-user"
          className="mt-1"
          autoComplete="username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Label htmlFor="login-pass" className="mt-2.5">
          Password
        </Label>
        <Input
          id="login-pass"
          className="mt-1"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Button type="submit" variant="primary" size="lg" className="mt-4" disabled={busy}>
          Sign in
        </Button>
        {error ? <div className="mt-3 font-mono text-[12px] text-ember">{error}</div> : null}
        <SsoHint />
        <div className="mt-2.5 text-center font-mono text-[11px] text-mist">
          reliquary {version.data?.version ?? "…"}
        </div>
      </form>
    </div>
  );
}
