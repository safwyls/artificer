package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/dwcon/internal/store"
)

// A Dragonwilds server end to end through the API: the config editor speaks
// dwconfig, rotate-admin-password works, and the command tier answers 501
// with the capability truth rather than 502.

const dwIni = `;METADATA=(Diff=true, UseCommands=true)
[/Script/Dominion.DedicatedServerSettings]
ServerName=Grimwood Bastion
DefaultWorldName=Ashenfall-Prime
OwnerId=owner-abc
AdminPassword=old-password
`

func newDragonwildsServer(t *testing.T, app *testApp, configPath string) int64 {
	t.Helper()
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "Grimwood", Game: "dragonwilds", Host: "127.0.0.1",
		Enabled: true, ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

func TestDragonwildsConfigEditor(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "DedicatedServer.ini")
	if err := os.WriteFile(file, []byte(dwIni), 0o644); err != nil {
		t.Fatal(err)
	}
	id := newDragonwildsServer(t, app, file)

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/config", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Settings []struct {
			Key, Value, Type string
		} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range res.Settings {
		if s.Key == "OwnerId" && s.Value == "owner-abc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("OwnerId not served: %+v", res.Settings)
	}

	rec = app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config", id),
		map[string]any{"changes": map[string]string{"ServerName": "Renamed Keep"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config: %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "ServerName=Renamed Keep") {
		t.Fatalf("edit not written:\n%s", data)
	}
	if !strings.Contains(string(data), ";METADATA=(Diff=true, UseCommands=true)") {
		t.Fatalf("metadata comment lost:\n%s", data)
	}
}

func TestDragonwildsRotateAdminPassword(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "DedicatedServer.ini")
	if err := os.WriteFile(file, []byte(dwIni), 0o644); err != nil {
		t.Fatal(err)
	}
	id := newDragonwildsServer(t, app, file)

	rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/config/rotate-admin-password", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Password) != 24 {
		t.Fatalf("password = %q, want 24 hex chars", res.Password)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "AdminPassword="+res.Password) {
		t.Fatal("new password not on disk")
	}
	if strings.Contains(string(data), "old-password") {
		t.Fatal("old password still on disk")
	}
}

func TestRotateAdminPasswordNeedsAConfigPath(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newDragonwildsServer(t, app, "")
	rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/config/rotate-admin-password", id), nil, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rotate without a config path: %d, want the setup-guidance 400", rec.Code)
	}
}

func TestDragonwildsCommandsAnswer501(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newDragonwildsServer(t, app, "")

	cases := []struct {
		path string
		body any
	}{
		{"broadcast", map[string]string{"message": "hi"}},
		{"kick", map[string]string{"playerUid": "u"}},
		{"ban", map[string]string{"playerUid": "u"}},
		{"unban", map[string]string{"playerUid": "u"}},
		{"save", nil},
		{"shutdown", map[string]any{"waitSeconds": 5, "message": "bye"}},
	}
	for _, tc := range cases {
		rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/%s", id, tc.path), tc.body, admin)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: %d %s, want 501", tc.path, rec.Code, rec.Body)
		}
	}
}

func TestDragonwildsFeaturesOnServerPayload(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newDragonwildsServer(t, app, "")
	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get server: %d", rec.Code)
	}
	var res struct {
		Game     string   `json:"game"`
		Features []string `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Game != "dragonwilds" {
		t.Fatalf("game = %q", res.Game)
	}
	want := []string{"pals", "saves", "logs"}
	if len(res.Features) != len(want) {
		t.Fatalf("features = %v, want %v", res.Features, want)
	}
	for i := range want {
		if res.Features[i] != want[i] {
			t.Fatalf("features = %v, want %v", res.Features, want)
		}
	}
}
