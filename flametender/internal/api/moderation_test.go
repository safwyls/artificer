package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The moderation surface over enshrouded_server.json: role groups behind
// the settings gate (they carry passwords), the ban list behind the
// moderation gate (it doesn't).

func esServerWithConfig(t *testing.T, app *testApp, body string) (id int64, file string) {
	t.Helper()
	dir := t.TempDir()
	file = filepath.Join(dir, "enshrouded_server.json")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return newEnshroudedServer(t, app, file), file
}

func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
}

type roleGroupDTO struct {
	Index                int    `json:"index"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	CanKickBan           bool   `json:"canKickBan"`
	CanAccessInventories bool   `json:"canAccessInventories"`
	CanEditBase          bool   `json:"canEditBase"`
	CanEditWorld         bool   `json:"canEditWorld"`
	ReservedSlots        int    `json:"reservedSlots"`
}

type rolesDTO struct {
	Groups          []roleGroupDTO `json:"groups"`
	Writable        bool           `json:"writable"`
	RestartRequired bool           `json:"restartRequired"`
}

func TestRolesRoundTrip(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, file := esServerWithConfig(t, app, esCfg)

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/config/roles", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get roles: %d %s", rec.Code, rec.Body)
	}
	var got rolesDTO
	decodeInto(t, rec, &got)
	if len(got.Groups) != 2 || got.Groups[0].Name != "Keepers" || !got.Groups[0].CanKickBan {
		t.Fatalf("groups = %+v", got.Groups)
	}
	// The console has to be able to say "restart to apply" without knowing
	// the game's reload semantics itself.
	if !got.RestartRequired || !got.Writable {
		t.Errorf("roles context wrong: %+v", got)
	}

	groups := got.Groups
	groups[1].CanEditWorld = true
	groups = append(groups, roleGroupDTO{Index: -1, Name: "Builders", Password: "build-pw", CanEditBase: true, ReservedSlots: 2})
	rec = app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config/roles", id),
		map[string]any{"groups": groups}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put roles: %d %s", rec.Code, rec.Body)
	}
	// The response is the fresh read, so the client resyncs to disk.
	decodeInto(t, rec, &got)
	if len(got.Groups) != 3 || got.Groups[2].Name != "Builders" || got.Groups[2].ReservedSlots != 2 {
		t.Fatalf("groups after write = %+v", got.Groups)
	}

	data, _ := os.ReadFile(file)
	// Everything outside userGroups is somebody else's edit surface.
	if !strings.Contains(string(data), "Grimwood Bastion") || !strings.Contains(string(data), "playerHealthFactor") {
		t.Fatalf("a role edit disturbed the rest of the config:\n%s", data)
	}
}

// The refusals that matter are the ones that would lock the operator out
// of their own server, and they have to happen before anything is written.
func TestRolesRefuseALockout(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, file := esServerWithConfig(t, app, esCfg)
	before, _ := os.ReadFile(file)

	cases := []struct {
		name   string
		groups []roleGroupDTO
	}{
		{"no kick/ban group at all", []roleGroupDTO{{Index: 1, Name: "Friends", Password: "join-pw"}}},
		{"an admin group anyone can join", []roleGroupDTO{{Index: 0, Name: "Keepers", CanKickBan: true}}},
		{"nothing at all", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config/roles", id),
				map[string]any{"groups": tc.groups}, admin)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body)
			}
		})
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(before) {
		t.Errorf("a refused role write still touched the file:\n%s", after)
	}
}

// Reading the roles is reading the join passwords, so the settings grant
// gates it in both directions — a moderator is not automatically someone
// who should hold every credential.
func TestRolesNeedTheSettingsGrant(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, _ := esServerWithConfig(t, app, esCfg)
	app.createUser(t, admin, "mod", "modpassword", "user", []string{"moderate"})
	mod := app.login(t, "mod", "modpassword")

	if rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/config/roles", id), nil, mod); rec.Code != http.StatusForbidden {
		t.Errorf("moderator reading roles: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config/roles", id), map[string]any{"groups": nil}, mod); rec.Code != http.StatusForbidden {
		t.Errorf("moderator writing roles: got %d, want 403", rec.Code)
	}
}

type banDTO struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
}

type pendingBanDTO struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Applied bool   `json:"applied"`
}

type bansDTO struct {
	Bans        []banDTO        `json:"bans"`
	Writable    bool            `json:"writable"`
	ObjectShape bool            `json:"objectShape"`
	Unreadable  int             `json:"unreadable"`
	Running     bool            `json:"running"`
	Pending     []pendingBanDTO `json:"pending"`
	Reverted    []pendingBanDTO `json:"reverted"`
}

const (
	banA = "76561198000000001"
	banB = "76561198000000002"
)

func TestBansRoundTrip(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, file := esServerWithConfig(t, app,
		`{"name": "Grimwood Bastion", "bannedAccounts": ["`+banA+`"], "userGroups": []}`)

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/bans", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get bans: %d %s", rec.Code, rec.Body)
	}
	var got bansDTO
	decodeInto(t, rec, &got)
	if len(got.Bans) != 1 || got.Bans[0].ID != banA {
		t.Fatalf("bans = %+v", got.Bans)
	}
	if got.ObjectShape || got.Unreadable != 0 {
		t.Errorf("file shape read wrong: %+v", got)
	}
	// No agent is configured, so nothing claims the game is up.
	if got.Running {
		t.Error("a server with no agent was reported running")
	}

	rec = app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/bans", id),
		map[string]any{"bans": []banDTO{{Index: -1, ID: banB}}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put bans: %d %s", rec.Code, rec.Body)
	}
	decodeInto(t, rec, &got)
	if len(got.Bans) != 1 || got.Bans[0].ID != banB {
		t.Fatalf("bans after write = %+v", got.Bans)
	}

	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), banA) {
		t.Errorf("the lifted ban is still on disk:\n%s", data)
	}
	if !strings.Contains(string(data), "Grimwood Bastion") {
		t.Errorf("a ban edit disturbed the rest of the config:\n%s", data)
	}
}

// Ids are the whole of a ban; a name pasted in by mistake bans nobody and
// says nothing about it until the player walks back in.
func TestBansRejectAnIdThatIsNotOne(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, file := esServerWithConfig(t, app, `{"bannedAccounts": ["`+banA+`"]}`)

	rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/bans", id),
		map[string]any{"bans": []banDTO{{Index: -1, ID: "Griefer"}}}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(file); !strings.Contains(string(data), banA) {
		t.Errorf("a refused ban write dropped the existing list:\n%s", data)
	}
}

// A moderator can work the ban list without holding the settings grant.
func TestBansNeedOnlyTheModerationGrant(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, _ := esServerWithConfig(t, app, `{"bannedAccounts": []}`)
	app.createUser(t, admin, "mod", "modpassword", "user", []string{"moderate"})
	app.createUser(t, admin, "watcher", "watcherpass", "user", nil)
	mod := app.login(t, "mod", "modpassword")
	watcher := app.login(t, "watcher", "watcherpass")

	if rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/bans", id), nil, mod); rec.Code != http.StatusOK {
		t.Errorf("moderator reading bans: got %d, want 200 (%s)", rec.Code, rec.Body)
	}
	rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/bans", id),
		map[string]any{"bans": []banDTO{{Index: -1, ID: banA}}}, mod)
	if rec.Code != http.StatusOK {
		t.Errorf("moderator writing bans: got %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/bans", id), nil, watcher); rec.Code != http.StatusForbidden {
		t.Errorf("a signed-in user with no grants: got %d, want 403", rec.Code)
	}
}

// The audit trail names who was banned and who was let back in. Account
// ids belong in it — unlike role passwords, they *are* the record.
func TestBanEditsAreAuditedByIdAndRolesNeverByPassword(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, _ := esServerWithConfig(t, app, `{"bannedAccounts": ["`+banA+`"], "userGroups": []}`)

	if rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/bans", id),
		map[string]any{"bans": []banDTO{{Index: -1, ID: banB}}}, admin); rec.Code != http.StatusOK {
		t.Fatalf("put bans: %d %s", rec.Code, rec.Body)
	}
	if rec := app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config/roles", id),
		map[string]any{"groups": []roleGroupDTO{{Index: -1, Name: "Keepers", Password: "s3cret-join-pw", CanKickBan: true}}}, admin); rec.Code != http.StatusOK {
		t.Fatalf("put roles: %d %s", rec.Code, rec.Body)
	}

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/audit", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit: %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "banned "+banB) || !strings.Contains(body, "unbanned "+banA) {
		t.Errorf("ban audit doesn't name the accounts:\n%s", body)
	}
	if !strings.Contains(body, "Keepers") {
		t.Errorf("role audit doesn't name the group:\n%s", body)
	}
	if strings.Contains(body, "s3cret-join-pw") {
		t.Errorf("a role password reached the audit trail:\n%s", body)
	}
}

func TestModerationEndpointsWithoutAConfigPath(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newEnshroudedServer(t, app, "")

	for _, path := range []string{"/config/roles", "/bans"} {
		rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d%s", id, path), nil, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s without a config path: got %d, want the setup-guidance 400", path, rec.Code)
		}
	}
}
