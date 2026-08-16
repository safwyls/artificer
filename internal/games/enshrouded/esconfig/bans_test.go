package esconfig

import (
	"strings"
	"testing"
)

const (
	steamA = "76561198000000001"
	steamB = "76561198000000002"
	steamC = "76561198000000003"
)

func bansOf(t *testing.T, src string) (Doc, []Ban, banShape, int) {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	bans, shape, unreadable := BansFrom(doc)
	return doc, bans, shape, unreadable
}

// The format the code and community both assume, and the one this editor
// defaults to when a file has nothing to say.
func TestBansReadBareStrings(t *testing.T) {
	_, bans, shape, unreadable := bansOf(t, `{"bannedAccounts": ["`+steamA+`", "`+steamB+`"]}`)

	if shape.object {
		t.Error("a list of strings was read as objects")
	}
	if unreadable != 0 {
		t.Errorf("unreadable = %d, want 0", unreadable)
	}
	if len(bans) != 2 || bans[0].ID != steamA || bans[1].Index != 1 {
		t.Errorf("bans read wrong: %+v", bans)
	}
	if bans[0].Name != "" {
		t.Error("a name was invented for a bare-string entry")
	}
}

// The other plausible shape. Which one a real server writes is the recon
// doc's open ledger row, so the editor follows the file rather than
// deciding for it.
func TestBansReadObjectsAndKeepTheirShape(t *testing.T) {
	src := `{"bannedAccounts": [{"steamId": "` + steamA + `", "name": "Grief", "bannedAt": 1234}]}`
	doc, bans, shape, _ := bansOf(t, src)

	if !shape.object || shape.idKey != "steamId" || shape.nameKey != "name" {
		t.Fatalf("shape read wrong: %+v", shape)
	}
	if len(bans) != 1 || bans[0].ID != steamA || bans[0].Name != "Grief" {
		t.Fatalf("bans read wrong: %+v", bans)
	}

	// Adding a ban must produce the same shape, not a bare string beside
	// the objects, and must not disturb the fields we don't model.
	SetBans(doc, append(bans, Ban{Index: -1, ID: steamB}))
	out := mustMarshal(t, doc)
	if !strings.Contains(out, `"bannedAt": 1234`) {
		t.Errorf("an unmodelled entry field was dropped:\n%s", out)
	}
	raw, _ := doc["bannedAccounts"].([]any)
	if len(raw) != 2 {
		t.Fatalf("wrote %d entries, want 2:\n%s", len(raw), out)
	}
	for i, e := range raw {
		if _, ok := e.(map[string]any); !ok {
			t.Errorf("entry %d is %T, want the file's object shape:\n%s", i, e, out)
		}
	}
	written, _, _ := BansFrom(doc)
	if written[1].ID != steamB {
		t.Errorf("new ban written wrong: %+v", written)
	}
}

func TestEmptyListDefaultsToBareStrings(t *testing.T) {
	doc, _, _, _ := bansOf(t, `{"bannedAccounts": []}`)
	SetBans(doc, []Ban{{Index: -1, ID: steamA}})

	out := mustMarshal(t, doc)
	if !strings.Contains(out, `"`+steamA+`"`) || strings.Contains(out, `"id"`) {
		t.Errorf("want a bare string entry, got:\n%s", out)
	}
}

// A file with no bannedAccounts key at all — every server that has never
// banned anyone.
func TestBansAbsentKeyReadsEmptyAndWritesTheKey(t *testing.T) {
	doc, bans, _, _ := bansOf(t, sample)
	if len(bans) != 0 {
		t.Fatalf("got %d bans from a file with no list", len(bans))
	}
	SetBans(doc, []Ban{{Index: -1, ID: steamA}})
	if !strings.Contains(mustMarshal(t, doc), steamA) {
		t.Error("the first ban didn't create the list")
	}
}

// The failure this whole shape-preserving design exists to avoid:
// silently lifting somebody's ban because the entry wasn't in a form the
// console understood.
func TestUnreadableEntriesSurviveAWrite(t *testing.T) {
	src := `{"bannedAccounts": ["` + steamA + `", {"opaque": true}, null]}`
	doc, bans, _, unreadable := bansOf(t, src)

	if unreadable != 2 {
		t.Fatalf("unreadable = %d, want 2", unreadable)
	}
	if len(bans) != 1 {
		t.Fatalf("readable bans = %+v, want just one", bans)
	}

	// Remove the one ban the console can see; the two it can't must stay.
	SetBans(doc, nil)
	out := mustMarshal(t, doc)
	if strings.Contains(out, steamA) {
		t.Errorf("the removed ban is still there:\n%s", out)
	}
	if !strings.Contains(out, "opaque") || !strings.Contains(out, "null") {
		t.Errorf("an entry the editor couldn't model was dropped:\n%s", out)
	}
}

func TestValidateBansRejectsWhatWouldBanNobody(t *testing.T) {
	cases := []struct {
		name string
		bans []Ban
		want string
	}{
		{"blank", []Ban{{ID: "  "}}, "needs an account id"},
		// The realistic mistake: pasting what's on screen instead of the id.
		{"a display name", []Ban{{ID: "Grief"}}, "17-digit"},
		{"a profile url", []Ban{{ID: "https://steamcommunity.com/id/grief"}}, "17-digit"},
		{"duplicate", []Ban{{ID: steamA}, {ID: steamA}}, "twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBans(tc.bans)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
	if err := ValidateBans([]Ban{{ID: steamA}, {ID: steamB}, {ID: steamC}}); err != nil {
		t.Errorf("a plain list of ids was refused: %v", err)
	}
	if err := ValidateBans(nil); err != nil {
		t.Errorf("an empty ban list should be fine: %v", err)
	}
}

func TestWriteBansRoundTripsThroughTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"name": "Embervale", "bannedAccounts": ["`+steamA+`"]}`)

	if err := WriteBans(dir, []Ban{{Index: 0, ID: steamA}, {Index: -1, ID: steamB}}); err != nil {
		t.Fatal(err)
	}
	res, err := ReadBans(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Bans) != 2 || res.Bans[1].ID != steamB {
		t.Fatalf("bans = %+v", res.Bans)
	}
	if res.ObjectShape || res.Unreadable != 0 || !res.Writable {
		t.Errorf("file context wrong: %+v", res)
	}

	// A rejected write leaves the file alone.
	if err := WriteBans(dir, []Ban{{ID: "not-an-id"}}); err == nil {
		t.Fatal("a junk id was accepted")
	}
	res, err = ReadBans(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Bans) != 2 {
		t.Errorf("the refused write changed the file: %+v", res.Bans)
	}
}

// Editing bans must not disturb the rest of the config — in particular
// the 64-bit durations that a careless round trip corrupts.
func TestWriteBansLeavesTheRestOfTheConfigAlone(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, sample)

	if err := WriteBans(path, []Ban{{Index: -1, ID: steamA}}); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out := mustMarshal(t, doc)
	if !strings.Contains(out, "1800000000000") {
		t.Errorf("dayTimeDuration corrupted by a ban edit:\n%s", out)
	}
	if groups := GroupsFrom(doc); len(groups) != 2 {
		t.Errorf("the role groups changed: %+v", groups)
	}
}
