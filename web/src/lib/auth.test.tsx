import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { AuthProvider, useAuth } from "./auth";
import { api, ApiError } from "./api";
import { renderWithProviders } from "../test/utils";

// A probe that renders exactly what the boot sequence decided.
function Probe() {
  const { username, loading, ssoError } = useAuth();
  if (loading) return <p>loading</p>;
  return (
    <div>
      <p>user:{username ?? "none"}</p>
      <p>sso-error:{ssoError ?? "none"}</p>
    </div>
  );
}

const me = {
  username: "ember@example.com",
  role: "",
  isAdmin: false,
  permissions: [],
};

describe("AuthProvider boot", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("uses an existing session without touching Cloudflare Access", async () => {
    vi.spyOn(api, "me").mockResolvedValue(me);
    const sso = vi.spyOn(api, "loginCloudflare");
    renderWithProviders(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    expect(await screen.findByText("user:ember@example.com")).toBeInTheDocument();
    // Signing in again when already signed in would be pointless traffic
    // on every page load.
    expect(sso).not.toHaveBeenCalled();
  });

  // The point of the whole feature: someone who cleared the Access policy
  // never sees a login form.
  it("signs in through Access when there is no session", async () => {
    const meSpy = vi
      .spyOn(api, "me")
      .mockRejectedValueOnce(new ApiError(401, "not authenticated"))
      .mockResolvedValueOnce(me);
    const sso = vi.spyOn(api, "loginCloudflare").mockResolvedValue({
      username: "ember@example.com",
      created: true,
    });
    renderWithProviders(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    expect(await screen.findByText("user:ember@example.com")).toBeInTheDocument();
    expect(sso).toHaveBeenCalled();
    // /me is re-read afterwards rather than trusting the login response:
    // role and permissions have one source.
    expect(meSpy).toHaveBeenCalledTimes(2);
  });

  // Not configured, or the request didn't come through the tunnel. Both
  // are ordinary — the password form is the answer, with nothing alarming
  // shown.
  it("falls through to the password form quietly", async () => {
    vi.spyOn(api, "me").mockRejectedValue(new ApiError(401, "not authenticated"));
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(404, "not found"));
    renderWithProviders(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    expect(await screen.findByText("user:none")).toBeInTheDocument();
    expect(screen.getByText("sso-error:none")).toBeInTheDocument();
  });

  // Access recognised them and this console refused. A blank password
  // form would be a dead end for someone who has no password.
  it("surfaces a refusal so the login page can explain it", async () => {
    vi.spyOn(api, "me").mockRejectedValue(new ApiError(401, "not authenticated"));
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(403, "account disabled"));
    renderWithProviders(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => expect(screen.getByText("sso-error:account disabled")).toBeInTheDocument());
    expect(screen.getByText("user:none")).toBeInTheDocument();
  });
});
