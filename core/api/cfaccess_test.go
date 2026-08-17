package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/safwyls/artificer/core/cfaccess"
	"github.com/safwyls/artificer/core/store"
)

// errForged stands for anything cfaccess refuses — a bad signature, the
// wrong audience, an expired token. The API layer must treat them all the
// same way: no session, no account, no detail leaked.
var errForged = errors.New("cloudflare access token rejected: signature is invalid")

// countUsers reads the users table directly; the admin API would work too
// but this keeps the assertion about state, not about another endpoint.
func (a *testApp) countUsers(t *testing.T) int {
	t.Helper()
	n, err := a.store.CountUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// fakeAccess stands in for Cloudflare's signature checking, which has its
// own tests in internal/cfaccess. What matters here is everything the API
// does *after* an identity is established.
type fakeAccess struct {
	identity *cfaccess.Identity
	err      error
}

func (f *fakeAccess) Verify(ctx context.Context, token string) (*cfaccess.Identity, error) {
	if token == "" {
		return nil, cfaccess.ErrNoToken
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}

func (f *fakeAccess) LogoutURL() string {
	return "https://emberhold.cloudflareaccess.com/cdn-cgi/access/logout"
}

// ssoLogin posts to the SSO route with an assertion header, the way a
// request proxied by Access arrives.
func (a *testApp) ssoLogin(t *testing.T, assertion string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/login/cloudflare", nil)
	if assertion != "" {
		req.Header.Set(cfaccess.HeaderName, assertion)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func withAccess(app *testApp, email string, admins ...string) *fakeAccess {
	f := &fakeAccess{identity: &cfaccess.Identity{Email: email, Subject: "sub-" + email}}
	app.api.Access = f
	app.api.AccessAdminEmails = admins
	return f
}

// The headline behaviour: someone who cleared the Access policy arrives
// with no account here, and gets one — with no permissions, because
// Access says who you are and never what you may do in this console.
func TestCloudflareLoginCreatesTheAccountOnFirstSignIn(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	withAccess(app, "ember@example.com")

	rec := app.ssoLogin(t, "assertion")
	if rec.Code != http.StatusOK {
		t.Fatalf("sso login: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Username string `json:"username"`
		Created  bool   `json:"created"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Username != "ember@example.com" || !out.Created {
		t.Fatalf("response = %+v", out)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie was issued")
	}

	user, err := app.store.GetUserByUsername(context.Background(), "ember@example.com")
	if err != nil {
		t.Fatalf("the account was not created: %v", err)
	}
	if user.IsAdmin() || len(user.Permissions) != 0 {
		t.Errorf("a new SSO account should hold nothing: %+v", user)
	}

	// The session works, and reports itself as SSO so the UI can offer
	// the sign-out that also ends the Access session.
	me := app.do(t, http.MethodGet, "/api/me", nil, cookies)
	if me.Code != http.StatusOK {
		t.Fatalf("/me: %d %s", me.Code, me.Body)
	}
	var meOut struct {
		Username     string `json:"username"`
		SSO          bool   `json:"sso"`
		SSOLogoutURL string `json:"ssoLogoutURL"`
	}
	json.Unmarshal(me.Body.Bytes(), &meOut)
	if meOut.Username != "ember@example.com" || !meOut.SSO || meOut.SSOLogoutURL == "" {
		t.Errorf("/me = %+v, want an SSO session with a logout URL", meOut)
	}

	// A second sign-in reuses the account rather than failing on the
	// unique username.
	again := app.ssoLogin(t, "assertion")
	if again.Code != http.StatusOK {
		t.Fatalf("second sso login: %d %s", again.Code, again.Body)
	}
	json.Unmarshal(again.Body.Bytes(), &out)
	if out.Created {
		t.Error("the second sign-in claimed to create the account again")
	}
}

// The lockout rescue: without it, the first person through Access lands
// in a console where nobody can grant them anything.
func TestCloudflareLoginAppliesTheAdminList(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	withAccess(app, "owner@example.com", "owner@example.com")

	if rec := app.ssoLogin(t, "assertion"); rec.Code != http.StatusOK {
		t.Fatalf("sso login: %d %s", rec.Code, rec.Body)
	}
	user, err := app.store.GetUserByUsername(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsAdmin() {
		t.Fatal("a listed operator did not get the admin role")
	}
}

// Re-applied on every sign-in, not only at creation: an operator who adds
// themselves to the list after their account already exists is exactly
// the person who needs rescuing.
func TestCloudflareLoginPromotesAnExistingAccount(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.createUser(t, admin, "owner@example.com", "password12345", "", []string{store.PermPower})
	withAccess(app, "owner@example.com", "owner@example.com")

	if rec := app.ssoLogin(t, "assertion"); rec.Code != http.StatusOK {
		t.Fatalf("sso login: %d %s", rec.Code, rec.Body)
	}
	user, err := app.store.GetUserByUsername(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsAdmin() {
		t.Error("an existing account was not promoted")
	}
	// Promotion must not quietly strip what they already had.
	if len(user.Permissions) != 1 || user.Permissions[0] != store.PermPower {
		t.Errorf("permissions = %v, want the existing grant kept", user.Permissions)
	}
}

// Disabling an account in the console outranks the identity provider —
// otherwise revoking someone would mean editing the Access policy.
func TestCloudflareLoginRefusesADisabledAccount(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := app.createUser(t, admin, "gone@example.com", "password12345", "", nil)
	rec := app.do(t, http.MethodPut, "/api/users/"+itoa(id),
		map[string]any{"role": "", "permissions": []string{}, "disabled": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body)
	}
	withAccess(app, "gone@example.com")

	if got := app.ssoLogin(t, "assertion"); got.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a disabled account", got.Code)
	}
}

// A machine cleared the Access policy. There is no console user it could
// be, and inventing one would be a back door with good manners.
func TestCloudflareLoginRefusesServiceTokens(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	f := withAccess(app, "")
	f.identity = &cfaccess.Identity{CommonName: "ci-runner.example"}

	rec := app.ssoLogin(t, "assertion")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a service token", rec.Code)
	}
	if n := app.countUsers(t); n != 1 {
		t.Errorf("users = %d, want only the bootstrap admin — a service token created an account", n)
	}
}

// The ordinary case for a request that didn't come through the tunnel:
// the frontend tries this route before showing the password form, and a
// LAN visitor falls through to it.
func TestCloudflareLoginWithoutAnAssertion(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	withAccess(app, "ember@example.com")

	if rec := app.ssoLogin(t, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 with no assertion", rec.Code)
	}
}

// Unconfigured, the route is simply absent — the frontend's silent
// attempt costs one 404 and nothing else.
func TestCloudflareLoginIsAbsentWhenUnconfigured(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)

	if rec := app.ssoLogin(t, "assertion"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when Access isn't configured", rec.Code)
	}
}

// An account created by Access has no password, and the stored sentinel
// must not be usable as one — that would turn the break-glass form into a
// way in for anyone who could guess it.
func TestSSOAccountsCannotUseThePasswordForm(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	withAccess(app, "ember@example.com")
	if rec := app.ssoLogin(t, "assertion"); rec.Code != http.StatusOK {
		t.Fatalf("sso login: %d %s", rec.Code, rec.Body)
	}

	for _, password := range []string{"", "!sso-only-no-password", "password"} {
		rec := app.do(t, http.MethodPost, "/api/login",
			map[string]string{"username": "ember@example.com", "password": password}, nil)
		if rec.Code == http.StatusOK {
			t.Fatalf("password %q signed in to an SSO-only account", password)
		}
	}
}

// A rejected assertion must not leak why, and must never fall back to
// creating an account.
func TestCloudflareLoginRejectsAnInvalidAssertion(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)
	f := withAccess(app, "attacker@example.com")
	f.err = errForged

	rec := app.ssoLogin(t, "forged")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if n := app.countUsers(t); n != 1 {
		t.Errorf("users = %d, want only the bootstrap admin", n)
	}
}
