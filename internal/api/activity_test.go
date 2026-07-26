package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestActivityReadableAuditAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	app.createUser(t, admin, "viewer", "viewerpassword", "user", nil)
	viewer := app.login(t, "viewer", "viewerpassword")

	if rec := app.do(t, "GET", base+"/activity", nil, viewer); rec.Code != http.StatusOK {
		t.Errorf("viewer activity: got %d, want 200", rec.Code)
	}
	if rec := app.do(t, "GET", base+"/audit", nil, viewer); rec.Code != http.StatusForbidden {
		t.Errorf("viewer audit: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "GET", base+"/audit", nil, admin); rec.Code != http.StatusOK {
		t.Errorf("admin audit: got %d, want 200", rec.Code)
	}
}

// Management actions leave audit rows; the row must attribute the action to
// the signed-in user and never carry secrets (config values are omitted).
func TestAuditRecordsActions(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)

	// A schedule create is a reliably-succeeding audited action (no game
	// server required, unlike broadcast/save).
	rec := app.do(t, "POST", base+"/schedules", map[string]any{
		"enabled": true, "days": []int{1}, "timeOfDay": "05:00", "warningMinutes": []int{5},
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create schedule: got %d (body %s)", rec.Code, rec.Body)
	}

	rec = app.do(t, "GET", base+"/audit", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit: got %d", rec.Code)
	}
	var res struct {
		Entries []struct {
			Username string `json:"username"`
			Action   string `json:"action"`
			Detail   string `json:"detail"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// (The test server was seeded through the store, not the API, so the
	// schedule create is the only audited action.)
	if len(res.Entries) != 1 || res.Entries[0].Action != "schedule-create" || res.Entries[0].Username != adminName {
		t.Fatalf("entries = %+v, want one schedule-create by %s", res.Entries, adminName)
	}
}

func TestContainerLogsGatedOnPower(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	app.createUser(t, admin, "nopower", "nopowerpass1", "user", nil)
	nopower := app.login(t, "nopower", "nopowerpass1")

	if rec := app.do(t, "GET", base+"/container/logs", nil, nopower); rec.Code != http.StatusForbidden {
		t.Errorf("logs without power: got %d, want 403", rec.Code)
	}
	// With the grant (admin implies it) but no docker configured, the
	// handler explains the missing setup rather than 500ing.
	if rec := app.do(t, "GET", base+"/container/logs", nil, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("logs without docker: got %d, want 400", rec.Code)
	}
}

func TestWatchdogToggle(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	app.createUser(t, admin, "plain", "plainpassword", "user", nil)
	plain := app.login(t, "plain", "plainpassword")

	if rec := app.do(t, "PUT", base+"/watchdog", map[string]any{"enabled": true}, plain); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin watchdog toggle: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "PUT", base+"/watchdog", map[string]any{"enabled": true}, admin); rec.Code != http.StatusOK {
		t.Fatalf("watchdog on: got %d (body %s)", rec.Code, rec.Body)
	}

	rec := app.do(t, "GET", base+"/automation", nil, admin)
	var auto struct {
		Watchdog struct {
			Enabled   bool `json:"enabled"`
			Available bool `json:"available"`
		} `json:"watchdog"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &auto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Enabled per the toggle; unavailable because no docker is configured.
	if !auto.Watchdog.Enabled || auto.Watchdog.Available {
		t.Errorf("watchdog = %+v, want enabled+unavailable", auto.Watchdog)
	}
}

func TestPublicStatusTokenFlow(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)

	// Off by default: no token, nothing served.
	if rec := app.do(t, "GET", "/api/public/status/nonexistent", nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown token: got %d, want 404", rec.Code)
	}

	rec := app.do(t, "PUT", base+"/public", map[string]any{"enabled": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.Enabled || len(res.Token) != 32 {
		t.Fatalf("res = %+v, want enabled with a 32-hex token", res)
	}

	// The public endpoint needs no session and must not leak connection
	// details or the server id.
	rec = app.do(t, "GET", "/api/public/status/"+res.Token, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("public status: got %d (body %s)", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "25575") {
		t.Errorf("public payload leaked connection details: %s", body)
	}

	// Disabling revokes the token.
	if rec := app.do(t, "PUT", base+"/public", map[string]any{"enabled": false}, admin); rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d", rec.Code)
	}
	if rec := app.do(t, "GET", "/api/public/status/"+res.Token, nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("revoked token: got %d, want 404", rec.Code)
	}
}
