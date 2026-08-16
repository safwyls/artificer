package esconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A config shaped like the game's own generated file (recon doc §2),
// including 64-bit nanosecond durations that would corrupt through a
// float64 round trip.
const sample = `{
    "name": "Embervale",
    "saveDirectory": "./savegame",
    "logDirectory": "./logs",
    "ip": "0.0.0.0",
    "queryPort": 15637,
    "slotCount": 8,
    "enableVoiceChat": false,
    "gameSettingsPreset": "Custom",
    "gameSettings": {
        "playerHealthFactor": 1.5,
        "dayTimeDuration": 1800000000000,
        "tombstoneMode": "AddBackpackMaterials"
    },
    "userGroups": [
        {"name": "Admins", "password": "oldadmin", "canKickBan": true, "reservedSlots": 1},
        {"name": "Friends", "password": "oldjoin", "canKickBan": false}
    ]
}`

func write(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "enshrouded_server.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The int64 durations are the round-trip landmine: float64 would render
// 1800000000000 as 1.8e+12 and the game's parser may or may not accept
// it. json.Number keeps the digits.
func TestRoundTripKeepsInt64Durations(t *testing.T) {
	doc, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	out, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "1800000000000") {
		t.Errorf("dayTimeDuration corrupted through the round trip:\n%s", out)
	}
}

func TestEnforceSetsIdentityAndPasswords(t *testing.T) {
	doc, _ := Parse([]byte(sample))
	admin, join := "newadmin", "newjoin"
	changed := Enforce(doc, Enforcement{
		ServerName: "Emberhold", QueryPort: 25637,
		AdminPassword: &admin, JoinPassword: &join,
	})
	if !changed {
		t.Fatal("nothing reported changed")
	}
	if doc["name"] != "Emberhold" {
		t.Errorf("name = %v", doc["name"])
	}
	if doc["queryPort"].(json.Number).String() != "25637" {
		t.Errorf("queryPort = %v", doc["queryPort"])
	}
	groups := doc["userGroups"].([]any)
	adm := groups[0].(map[string]any)
	frd := groups[1].(map[string]any)
	if adm["password"] != "newadmin" || frd["password"] != "newjoin" {
		t.Errorf("passwords = %v / %v", adm["password"], frd["password"])
	}
	// Names belong to the operator; matching is by capability.
	if adm["name"] != "Admins" || frd["name"] != "Friends" {
		t.Errorf("group names were touched: %v / %v", adm["name"], frd["name"])
	}
	// Untouched keys survive.
	if doc["slotCount"].(json.Number).String() != "8" {
		t.Errorf("slotCount was disturbed: %v", doc["slotCount"])
	}

	// A second pass with the same values is a no-op — this is what keeps
	// every game start from rewriting the file.
	if Enforce(doc, Enforcement{ServerName: "Emberhold", QueryPort: 25637, AdminPassword: &admin, JoinPassword: &join}) {
		t.Error("an already-enforced config reported changes")
	}
}

// A config with no matching group gets one seeded rather than leaving the
// configured credential silently unenforced.
func TestEnforceSeedsMissingGroups(t *testing.T) {
	doc, _ := Parse([]byte(`{"name": "x", "userGroups": []}`))
	admin := "adm"
	Enforce(doc, Enforcement{AdminPassword: &admin})
	groups := doc["userGroups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %v", groups)
	}
	g := groups[0].(map[string]any)
	if g["canKickBan"] != true || g["password"] != "adm" {
		t.Errorf("seeded admin group = %v", g)
	}
}

func TestSeedIsPrivateFromTheFirstSecond(t *testing.T) {
	admin, join := "adm", "join"
	doc := Seed(Enforcement{ServerName: "Emberhold", QueryPort: 25637, AdminPassword: &admin, JoinPassword: &join})
	if doc["name"] != "Emberhold" {
		t.Errorf("name = %v", doc["name"])
	}
	groups := doc["userGroups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("want an admin and a join group, got %v", groups)
	}
	// The whole point of seeding before first boot: the game's own
	// generated default is an open server.
	if groups[1].(map[string]any)["password"] != "join" {
		t.Errorf("join group = %v", groups[1])
	}
}

func TestWriteKeepsABackupAndSwapsAtomically(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, sample)
	doc, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	doc["name"] = "Renamed"
	if err := Write(p, doc); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(p + ".bak")
	if err != nil {
		t.Fatalf("no .bak kept: %v", err)
	}
	if !strings.Contains(string(bak), `"Embervale"`) {
		t.Errorf(".bak does not hold the previous contents")
	}
	if reread, _ := Load(p); reread["name"] != "Renamed" {
		t.Errorf("rewrite did not land: %v", reread["name"])
	}
}

func TestValidateRejectsWhatWouldBrickTheServer(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"malformed", `{"name": `},
		{"port out of range", `{"queryPort": 70000}`},
		{"slot count over cap", `{"slotCount": 32}`},
		{"userGroups wrong shape", `{"userGroups": "nope"}`},
	} {
		if err := Validate([]byte(tc.body)); err == nil {
			t.Errorf("%s: accepted", tc.name)
		}
	}
	if err := Validate([]byte(sample)); err != nil {
		t.Errorf("the sample config was rejected: %v", err)
	}
}

// The flat editor view: top-level scalars under "server", gameSettings
// under their own section with dotted keys, arrays absent.
func TestEditorReadFlattens(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, sample)
	res, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Setting{}
	for _, s := range res.Settings {
		byKey[s.Key] = s
	}
	if s := byKey["name"]; s.Type != "string" || s.Value != "Embervale" || s.Section != "server" {
		t.Errorf("name = %+v", s)
	}
	if s := byKey["queryPort"]; s.Type != "int" || s.Value != "15637" {
		t.Errorf("queryPort = %+v", s)
	}
	if s := byKey["gameSettings.playerHealthFactor"]; s.Type != "float" || s.Value != "1.5" || s.Section != "gameSettings" {
		t.Errorf("playerHealthFactor = %+v", s)
	}
	if s := byKey["gameSettings.dayTimeDuration"]; s.Type != "int" || s.Value != "1800000000000" {
		t.Errorf("dayTimeDuration = %+v", s)
	}
	if _, ok := byKey["userGroups"]; ok {
		t.Error("userGroups leaked into the flat editor — role passwords do not belong there")
	}
}

func TestEditorWriteValidatesAndNeverAdds(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, sample)

	if err := WriteChanges(dir, map[string]string{"slotCount": "12", "gameSettings.playerHealthFactor": "2"}); err != nil {
		t.Fatal(err)
	}
	doc, _ := Load(p)
	if doc["slotCount"].(json.Number).String() != "12" {
		t.Errorf("slotCount = %v", doc["slotCount"])
	}
	gs := doc["gameSettings"].(map[string]any)
	if gs["playerHealthFactor"].(json.Number).String() != "2" {
		t.Errorf("playerHealthFactor = %v", gs["playerHealthFactor"])
	}

	if err := WriteChanges(dir, map[string]string{"enableVoiceChat": "definitely"}); err == nil {
		t.Error("a non-bool value for a bool key was accepted")
	}
	if err := WriteChanges(dir, map[string]string{"madeUpKey": "1"}); err == nil {
		t.Error("an unknown key was accepted — the never-add policy is gone")
	}
}

func TestRotateAdminPasswordTargetsTheKickBanGroup(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, sample)
	if err := RotateAdminPassword(dir, "rotated"); err != nil {
		t.Fatal(err)
	}
	doc, _ := Load(p)
	groups := doc["userGroups"].([]any)
	if groups[0].(map[string]any)["password"] != "rotated" {
		t.Errorf("admin password = %v", groups[0])
	}
	if groups[1].(map[string]any)["password"] != "oldjoin" {
		t.Errorf("the join password moved too: %v", groups[1])
	}
}
