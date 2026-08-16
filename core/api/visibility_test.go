package api_test

import (
	"net/http"
	"testing"
)

// makeMember creates a non-admin account and returns its session, for checking
// the half of visibility that admins don't experience.
func makeMember(t *testing.T, app *testApp, admin []*http.Cookie) []*http.Cookie {
	t.Helper()
	rec := app.do(t, "POST", "/api/users", map[string]any{
		"username": "member", "password": "member-password-1", "role": "member",
	}, admin)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create member: got %d (body %s)", rec.Code, rec.Body)
	}
	return app.login(t, "member", "member-password-1")
}

func newServerForVisibility(t *testing.T, app *testApp, admin []*http.Cookie) {
	t.Helper()
	rec := app.do(t, "POST", "/api/servers", map[string]any{
		"name": "s1", "host": "10.0.0.1", "rconPort": 25575, "rconPassword": "x",
	}, admin)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create server: got %d (body %s)", rec.Code, rec.Body)
	}
}

func TestVisibilityIsAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	member := makeMember(t, app, admin)

	// The list of who asked to be hidden is itself the sort of thing the
	// hiding is meant to keep quiet, so reading it is admin-only too.
	if rec := app.do(t, "GET", "/api/servers/1/visibility", nil, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member GET visibility: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"inventory"},
	}, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member PUT visibility: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "GET", "/api/servers/1/visibility", nil, admin); rec.Code != http.StatusOK {
		t.Fatalf("admin GET visibility: got %d, want 200 (body %s)", rec.Code, rec.Body)
	}
}

func TestVisibilityPutReplacesWholeState(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	const uid = "11111111-1111-1111-1111-111111111111"

	app.do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{},
		"players":        map[string][]string{uid: {"inventory"}},
	}, admin)

	// An omitted player is one nobody is hiding any more — otherwise a hide
	// could only ever be added, never lifted.
	app.do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{},
		"players":        map[string][]string{},
	}, admin)

	rec := app.do(t, "GET", "/api/servers/1/visibility", nil, admin)
	got := decodeMap(t, rec)
	players, _ := got["players"].(map[string]any)
	if len(players) != 0 {
		t.Fatalf("player hides survived a replacing PUT: %v", players)
	}
}
