import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError, api } from "../lib/api";
import { AuthProvider } from "../lib/auth";
import { Login } from "./Login";
import { renderWithProviders } from "../test/utils";

const show = () =>
  renderWithProviders(
    <AuthProvider>
      <Login />
    </AuthProvider>,
  );

describe("Login", () => {
  beforeEach(() => {
    vi.spyOn(api, "version").mockResolvedValue({ version: "v1.4.2" });
  });
  afterEach(() => vi.restoreAllMocks());

  const noSession = () => vi.spyOn(api, "me").mockRejectedValue(new ApiError(401, "unauthorized"));

  it("names the build, so a bug report can say which one", async () => {
    noSession();
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(404, "not found"));
    show();
    expect(await screen.findByText("reliquary v1.4.2")).toBeInTheDocument();
  });

  it("says when this server has no Cloudflare Access configuration", async () => {
    noSession();
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(404, "not found"));
    show();
    expect(await screen.findByText(/not configured on this server/)).toBeInTheDocument();
    expect(screen.getByText(/CF_ACCESS_TEAM_DOMAIN and CF_ACCESS_AUD/)).toBeInTheDocument();
  });

  it("says when the request reached reliquary without an Access assertion", async () => {
    noSession();
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(401, "unauthorized"));
    show();
    expect(await screen.findByText(/didn't arrive with a Cloudflare Access assertion/)).toBeInTheDocument();
  });

  it("passes any other Access failure through in the server's own words", async () => {
    noSession();
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(403, "this account is disabled"));
    show();
    expect(
      await screen.findByText(/Cloudflare Access sign-in failed: this account is disabled/),
    ).toBeInTheDocument();
  });

  it("signs in with a password and shows the server's refusal when it fails", async () => {
    noSession();
    vi.spyOn(api, "loginCloudflare").mockRejectedValue(new ApiError(404, "not found"));
    const login = vi.spyOn(api, "login").mockRejectedValue(new ApiError(401, "wrong password"));
    show();
    await userEvent.type(await screen.findByLabelText("Username"), "safwyl");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByText("wrong password")).toBeInTheDocument();
    expect(login).toHaveBeenCalledWith("safwyl", "hunter2");
  });
});

