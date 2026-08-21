package dragonwilds_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

// companionRecordJSON is a character record shaped like the game's client
// save, with the guid given in its base64url spelling (the same 16 bytes
// the world save renders as hex).
func companionRecordJSON(name, guidB64 string) map[string]any {
	return map[string]any{
		"Version": 17,
		"meta_data": map[string]any{
			"char_guid": guidB64,
			"char_name": name,
			"worlds_playtime": map[string]any{
				// The fixture world's guid, hyphenated lowercase.
				"ca220b25-4bb4-4040-a066-6fb7646ed7fa": 3600.0,
			},
		},
		"SaveCount": 5,
		"Character": map[string]any{
			"Health": map[string]any{"CurrentValue": 71.0},
			"LastAccessibleLocation": map[string]any{
				"Position": "X=1000.000 Y=2000.000 Z=300.000",
			},
		},
		"Inventory": map[string]any{
			"0": map[string]any{"GUID": "aaaaaaaaaaaaaaaaaaaaaa", "ItemData": "P3_Aq0nAXu5dlFuBNGgyaw"},
		},
		"Skills": map[string]any{
			"Skills": []any{map[string]any{"Id": "4zYUGF5u_0KbMLkWJmmBbQ", "Xp": 13363}},
		},
	}
}

// TestCompanionFlow drives the whole loop the companion app uses:
// admin enables sharing, the app validates its token, pushes a record,
// and the world endpoint serves the merged character.
func TestCompanionFlow(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	worldPath := worldServer(t, app, realSaveDir(t))
	base := worldPath[:len(worldPath)-len("/world")]

	// Off by default: pushing against a nonexistent token is a plain 404.
	if rec := app.Do(t, "POST", "/api/public/companion/nosuchtoken/character", companionRecordJSON("Aldra", "abcdefghijklmnopqrstuv"), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("push without sharing enabled: got %d, want 404", rec.Code)
	}

	// Admin turns sharing on and gets the token.
	rec := app.Do(t, "PUT", base+"/companion", map[string]any{"enabled": true}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: got %d: %s", rec.Code, rec.Body)
	}
	var enabled struct {
		Enabled bool
		Token   string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &enabled); err != nil || !enabled.Enabled || enabled.Token == "" {
		t.Fatalf("enable answer = %s", rec.Body)
	}

	// The companion's config check: the token answers with the server name.
	rec = app.Do(t, "GET", "/api/public/companion/"+enabled.Token, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ping: got %d", rec.Code)
	}

	// Push a character. The guid decodes to 16 bytes.
	guidB64 := "ESIzRFVmd4iZqrvM3e7_AA" // 0x1122...ff00 base64url
	rec = app.Do(t, "POST", "/api/public/companion/"+enabled.Token+"/character", companionRecordJSON("Aldra", guidB64), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("push: got %d: %s", rec.Code, rec.Body)
	}

	// Junk is rejected with the reason, not stored.
	rec = app.Do(t, "POST", "/api/public/companion/"+enabled.Token+"/character", map[string]any{"TriggerData": 1}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("junk push: got %d, want 400", rec.Code)
	}

	// The world now carries the shared character, merged by guid.
	rec = app.Do(t, "GET", worldPath, nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("world: got %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		World struct {
			Players []dwsave.PlayerCharacter `json:"players"`
		} `json:"world"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.World.Players) != 1 {
		t.Fatalf("players = %+v, want the shared character", out.World.Players)
	}
	p := out.World.Players[0]
	if p.CharName != "Aldra" || p.SaveCount != 5 || len(p.Skills) != 1 || len(p.Inventory) != 1 {
		t.Errorf("merged character = %+v", p)
	}
	if p.SharedAt == nil {
		t.Error("SharedAt not stamped on a companion-shared record")
	}
	if p.PlaytimeHours != 1 {
		t.Errorf("PlaytimeHours = %v, want the fixture world's entry resolved", p.PlaytimeHours)
	}
	if p.Position == nil || p.Position.X != 1000 {
		t.Errorf("Position = %+v, want the record's own (no transform in the fixture)", p.Position)
	}

	// Disabling drops the token and everything it delivered.
	if rec := app.Do(t, "PUT", base+"/companion", map[string]any{"enabled": false}, admin); rec.Code != http.StatusOK {
		t.Fatalf("disable: got %d", rec.Code)
	}
	if rec := app.Do(t, "POST", "/api/public/companion/"+enabled.Token+"/character", companionRecordJSON("Aldra", guidB64), nil); rec.Code != http.StatusNotFound {
		t.Errorf("push after disable: got %d, want 404", rec.Code)
	}
	rec = app.Do(t, "GET", worldPath, nil, admin)
	out.World.Players = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.World.Players) != 0 {
		t.Errorf("players after disable = %+v, want none", out.World.Players)
	}
}

// TestCompanionDownload pins the exe hand-out: token-gated, served as an
// attachment when the deployment bundles it, honest when it doesn't.
func TestCompanionDownload(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	worldPath := worldServer(t, app, realSaveDir(t))
	base := worldPath[:len(worldPath)-len("/world")]

	rec := app.Do(t, "PUT", base+"/companion", map[string]any{"enabled": true}, admin)
	var enabled struct{ Token string }
	if err := json.Unmarshal(rec.Body.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}

	// No exe wired (the harness default): a named refusal, not a 500.
	rec = app.Do(t, "GET", "/api/public/companion/"+enabled.Token+"/download", nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "without the companion app") {
		t.Fatalf("download without a bundle: got %d %s", rec.Code, rec.Body)
	}

	// With the bundle present, the exe streams as an attachment.
	exe := filepath.Join(t.TempDir(), "artificer-companion.exe")
	if err := os.WriteFile(exe, []byte("MZfake-exe-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	dwAPIUnderTest.SetCompanionExe(exe)
	rec = app.Do(t, "GET", "/api/public/companion/"+enabled.Token+"/download", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: got %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "artificer-companion.exe") {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Body.String() != "MZfake-exe-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}

	// The wrong token gets nothing, bundle or not.
	if rec := app.Do(t, "GET", "/api/public/companion/wrongtoken/download", nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("download with bad token: got %d", rec.Code)
	}
}

// TestCompanionAdminOnly pins the management surface to admins.
func TestCompanionAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	worldPath := worldServer(t, app, realSaveDir(t))
	base := worldPath[:len(worldPath)-len("/world")]
	app.CreateUser(t, admin, "peon", "peonpassword1", "user", nil)
	peon := app.Login(t, "peon", "peonpassword1")

	for _, tc := range []struct{ method, path string }{
		{"GET", base + "/companion"},
		{"PUT", base + "/companion"},
	} {
		if rec := app.Do(t, tc.method, tc.path, map[string]any{"enabled": true}, peon); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: got %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

// TestCompanionRecordCap pins the per-server bound: a leaked token cannot
// grow the inbox without limit.
func TestCompanionRecordCap(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	worldPath := worldServer(t, app, realSaveDir(t))
	base := worldPath[:len(worldPath)-len("/world")]

	rec := app.Do(t, "PUT", base+"/companion", map[string]any{"enabled": true}, admin)
	var enabled struct{ Token string }
	if err := json.Unmarshal(rec.Body.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}
	push := func(i int) int {
		// 22-char base64url guids, distinct per i in the leading bytes —
		// trailing-character variation would alias under canonical
		// decoding (four trailing bits per 22-char group).
		guid := fmt.Sprintf("%02xAAAAAAAAAAAAAAAAAAAA", i)[:22]
		r := app.Do(t, "POST", "/api/public/companion/"+enabled.Token+"/character", companionRecordJSON("C", guid), nil)
		return r.Code
	}
	for i := 0; i < 16; i++ {
		if code := push(i); code != http.StatusOK {
			t.Fatalf("push %d: got %d", i, code)
		}
	}
	if code := push(16); code != http.StatusConflict {
		t.Errorf("push past the cap: got %d, want 409", code)
	}
	// A re-push of a known character still lands.
	if code := push(3); code != http.StatusOK {
		t.Errorf("re-push of known character: got %d, want 200", code)
	}
}
