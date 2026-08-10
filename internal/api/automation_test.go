package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/safwyls/wildskeeper/internal/store"
)

// createTestServer inserts a server row directly; automation endpoints
// don't need a reachable game server.
func createTestServer(t *testing.T, app *testApp) int64 {
	t.Helper()
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

func TestScheduleCRUD(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)

	rec := app.do(t, "POST", base+"/schedules", map[string]any{
		"enabled": true, "days": []int{5, 1, 1}, "timeOfDay": "05:00", "warningMinutes": []int{5, 15, 5},
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d (body %s)", rec.Code, rec.Body)
	}
	var created struct {
		ID             int64  `json:"id"`
		Days           []int  `json:"days"`
		WarningMinutes []int  `json:"warningMinutes"`
		NextRunAt      string `json:"nextRunAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Canonicalized: days deduped ascending, warnings deduped descending.
	if len(created.Days) != 2 || created.Days[0] != 1 || created.Days[1] != 5 {
		t.Errorf("days = %v, want [1 5]", created.Days)
	}
	if len(created.WarningMinutes) != 2 || created.WarningMinutes[0] != 15 {
		t.Errorf("warnings = %v, want [15 5]", created.WarningMinutes)
	}
	if created.NextRunAt == "" {
		t.Error("enabled schedule should report nextRunAt")
	}

	rec = app.do(t, "PUT", base+"/schedules/"+itoa(created.ID), map[string]any{
		"enabled": false, "days": []int{2}, "timeOfDay": "23:30", "warningMinutes": []int{},
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: got %d (body %s)", rec.Code, rec.Body)
	}

	rec = app.do(t, "GET", base+"/automation", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("automation: got %d", rec.Code)
	}
	var auto struct {
		Schedules []struct {
			TimeOfDay string  `json:"timeOfDay"`
			Enabled   bool    `json:"enabled"`
			NextRunAt *string `json:"nextRunAt"`
		} `json:"schedules"`
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &auto); err != nil {
		t.Fatalf("decode automation: %v", err)
	}
	if len(auto.Schedules) != 1 || auto.Schedules[0].TimeOfDay != "23:30" || auto.Schedules[0].Enabled {
		t.Errorf("schedules = %+v, want the updated one", auto.Schedules)
	}
	if auto.Schedules[0].NextRunAt != nil {
		t.Error("disabled schedule must not report nextRunAt")
	}
	if auto.Timezone == "" {
		t.Error("automation payload should carry the host timezone")
	}

	if rec := app.do(t, "DELETE", base+"/schedules/"+itoa(created.ID), nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", rec.Code)
	}
}

func TestScheduleValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	path := "/api/servers/" + itoa(id) + "/schedules"

	bad := []map[string]any{
		{"days": []int{1}, "timeOfDay": "25:00"},                             // bad hour
		{"days": []int{1}, "timeOfDay": "0500"},                              // not HH:MM
		{"days": []int{}, "timeOfDay": "05:00"},                              // no days
		{"days": []int{7}, "timeOfDay": "05:00"},                             // day out of range
		{"days": []int{1}, "timeOfDay": "05:00", "warningMinutes": []int{0}}, // zero warning
	}
	for _, body := range bad {
		if rec := app.do(t, "POST", path, body, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("body %v: got %d, want 400", body, rec.Code)
		}
	}
}

func TestAutomationAdminGating(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	app.createUser(t, admin, "player", "playerpassword", "user", nil)
	player := app.login(t, "player", "playerpassword")

	// Anyone signed in may read the schedules ("when's the restart?"),
	// but the Discord section is admin-only and must be absent.
	rec := app.do(t, "GET", base+"/automation", nil, player)
	if rec.Code != http.StatusOK {
		t.Fatalf("player automation read: got %d, want 200", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := payload["discord"]; ok {
		t.Error("discord config leaked to a non-admin")
	}

	writes := []struct{ method, path string }{
		{"POST", base + "/schedules"},
		{"PUT", base + "/schedules/1"},
		{"DELETE", base + "/schedules/1"},
		{"PUT", base + "/discord"},
		{"DELETE", base + "/discord"},
		{"POST", base + "/discord/test"},
	}
	for _, wr := range writes {
		if rec := app.do(t, wr.method, wr.path, map[string]any{}, player); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: got %d, want 403", wr.method, wr.path, rec.Code)
		}
	}
}

func TestDiscordConfigNeverEchoesURL(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	const secret = "https://discord.com/api/webhooks/42/super-secret-token"

	rec := app.do(t, "PUT", base+"/discord", map[string]any{
		"webhookUrl": secret, "enabled": true, "onStatus": true, "onPlayers": true, "onRestarts": false,
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put discord: got %d (body %s)", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Error("PUT response echoed the webhook URL")
	}

	rec = app.do(t, "GET", base+"/automation", nil, admin)
	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Error("automation payload leaked the webhook URL")
	}
	var auto struct {
		Discord struct {
			Configured bool `json:"configured"`
			Enabled    bool `json:"enabled"`
			OnRestarts bool `json:"onRestarts"`
		} `json:"discord"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &auto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !auto.Discord.Configured || !auto.Discord.Enabled || auto.Discord.OnRestarts {
		t.Errorf("discord config mismatch: %+v", auto.Discord)
	}

	// Toggle-only update (no URL) keeps the stored webhook.
	rec = app.do(t, "PUT", base+"/discord", map[string]any{"enabled": false, "onStatus": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle-only put: got %d (body %s)", rec.Code, rec.Body)
	}

	if rec := app.do(t, "DELETE", base+"/discord", nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete discord: got %d", rec.Code)
	}
	// With nothing stored, a toggle-only update has no secret to keep.
	if rec := app.do(t, "PUT", base+"/discord", map[string]any{"enabled": true}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("toggle-only with nothing stored: got %d, want 400", rec.Code)
	}
}

func TestDiscordURLValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	path := "/api/servers/" + itoa(id) + "/discord"

	for _, url := range []string{
		"http://discord.com/api/webhooks/1/x",       // not https
		"https://evil.example.com/api/webhooks/1/x", // wrong host
		"https://discord.com/other/1/x",             // wrong path
		"not a url at all ://",
	} {
		rec := app.do(t, "PUT", path, map[string]any{"webhookUrl": url, "enabled": true}, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("url %q: got %d, want 400", url, rec.Code)
		}
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
