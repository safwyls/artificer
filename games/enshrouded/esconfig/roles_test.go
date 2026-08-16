package esconfig

import (
	"strings"
	"testing"
)

// A file with a group carrying a key this editor doesn't model, which is
// the realistic case: the game added canEditWorld in 2025 and will add
// more.
const groupsSample = `{
    "name": "Embervale",
    "queryPort": 15637,
    "userGroups": [
        {"name": "Admins", "password": "adminpw", "canKickBan": true, "canAccessInventories": true, "canEditBase": true, "canExtendBase": true, "canEditWorld": true, "reservedSlots": 1, "canSummonMeteors": true},
        {"name": "Friends", "password": "joinpw", "canKickBan": false, "canEditBase": true, "reservedSlots": 0}
    ]
}`

func groupsOf(t *testing.T, src string) (Doc, []Group) {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return doc, GroupsFrom(doc)
}

func TestGroupsFromReadsCapabilitiesAndSlots(t *testing.T) {
	_, groups := groupsOf(t, groupsSample)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	admin := groups[0]
	if admin.Name != "Admins" || admin.Password != "adminpw" || !admin.CanKickBan {
		t.Errorf("admin group read wrong: %+v", admin)
	}
	if admin.ReservedSlots != 1 {
		t.Errorf("reservedSlots = %d, want 1", admin.ReservedSlots)
	}
	// Index is how a write finds the original element again.
	if admin.Index != 0 || groups[1].Index != 1 {
		t.Errorf("indices = %d,%d, want 0,1", admin.Index, groups[1].Index)
	}
	// A key the file omits is false, not an error: the game defaults it.
	if groups[1].CanEditWorld {
		t.Error("an absent canEditWorld should read false")
	}
}

// The whole reason groups carry an Index: the game has twice grown new
// per-group fields, and an editor that rewrote each group from its own
// struct would silently delete every field it was written before.
func TestSetGroupsKeepsUnmodelledKeys(t *testing.T) {
	doc, groups := groupsOf(t, groupsSample)
	groups[0].Name = "Wardens"
	SetGroups(doc, groups)

	out, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "canSummonMeteors") {
		t.Errorf("an unmodelled group key was dropped:\n%s", out)
	}
	if !strings.Contains(string(out), "Wardens") {
		t.Errorf("the rename didn't land:\n%s", out)
	}
	// reservedSlots must stay a JSON number, not become "1".
	if strings.Contains(string(out), `"reservedSlots": "1"`) {
		t.Errorf("reservedSlots was stringified:\n%s", out)
	}
}

// A group added in the console has no original element to merge into.
func TestSetGroupsWritesNewGroupsFromScratch(t *testing.T) {
	doc, groups := groupsOf(t, groupsSample)
	groups = append(groups, Group{Index: -1, Name: "Builders", Password: "buildpw", CanEditBase: true, CanExtendBase: true})
	SetGroups(doc, groups)

	written := GroupsFrom(doc)
	if len(written) != 3 {
		t.Fatalf("got %d groups, want 3", len(written))
	}
	got := written[2]
	if got.Name != "Builders" || got.Password != "buildpw" || !got.CanExtendBase || got.CanKickBan {
		t.Errorf("new group written wrong: %+v", got)
	}
}

// Order is the caller's: the console can reorder groups, and the game
// reads the list in order.
func TestSetGroupsHonoursCallerOrder(t *testing.T) {
	doc, groups := groupsOf(t, groupsSample)
	SetGroups(doc, []Group{groups[1], groups[0]})

	written := GroupsFrom(doc)
	if written[0].Name != "Friends" || written[1].Name != "Admins" {
		t.Errorf("order not honoured: %q, %q", written[0].Name, written[1].Name)
	}
	if !strings.Contains(mustMarshal(t, doc), "canSummonMeteors") {
		t.Error("reordering lost an unmodelled key")
	}
}

func mustMarshal(t *testing.T, doc Doc) string {
	t.Helper()
	out, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestValidateGroupsRefusesTheSelfInflictedWounds(t *testing.T) {
	admin := Group{Index: 0, Name: "Admins", Password: "adminpw", CanKickBan: true}
	friends := Group{Index: 1, Name: "Friends", Password: "joinpw"}

	cases := []struct {
		name   string
		groups []Group
		want   string
	}{
		{"empty list", nil, "at least one"},
		// The one that matters most: kick/ban is only reachable from inside
		// the game, by someone who joined with an admin group's password.
		// Delete every such group and the deployment has no moderation at
		// all, and the console has no way to say so afterwards.
		{"no admin group", []Group{friends}, "kick/ban"},
		// An empty password on an admin group isn't a weak credential, it's
		// no credential: anyone who finds the server joins with kick/ban.
		{"admin without a password", []Group{{Name: "Admins", CanKickBan: true}}, "empty one"},
		{"nameless group", []Group{admin, {Index: 1, Name: "  "}}, "needs a name"},
		{"duplicate names", []Group{admin, {Index: 1, Name: "admins", Password: "other"}}, "both called"},
		// The game picks a joining player's group by password, so a shared
		// one makes the resulting permissions a coin flip.
		{"duplicate passwords", []Group{admin, {Index: 1, Name: "Friends", Password: "adminpw"}}, "shares its password"},
		{"absurd reserved slots", []Group{admin, {Index: 1, Name: "Friends", ReservedSlots: 99}}, "0-16"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGroups(tc.groups)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}

	if err := ValidateGroups([]Group{admin, friends}); err != nil {
		t.Errorf("a sane pair of groups was refused: %v", err)
	}
	// A password-less *non*-admin group is a deliberate choice — an open
	// server — and the game's own semantics for it.
	if err := ValidateGroups([]Group{admin, {Index: 1, Name: "Everyone"}}); err != nil {
		t.Errorf("an open join group was refused: %v", err)
	}
}

func TestWriteGroupsValidatesBeforeTouchingTheFile(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, groupsSample)

	// Dropping the admin group must fail, and the file must be untouched —
	// a half-applied role write is a locked-out server.
	if err := WriteGroups(dir, []Group{{Index: 1, Name: "Friends", Password: "joinpw"}}); err == nil {
		t.Fatal("dropping the last admin group was accepted")
	}
	res, err := ReadGroups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 2 || !res.Groups[0].CanKickBan {
		t.Errorf("the refused write still changed the file: %+v", res.Groups)
	}

	groups := res.Groups
	groups[1].CanEditWorld = true
	if err := WriteGroups(dir, groups); err != nil {
		t.Fatal(err)
	}
	res, err = ReadGroups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Groups[1].CanEditWorld {
		t.Error("the accepted write didn't land")
	}
	if res.Path == "" || !res.Writable {
		t.Errorf("file context wrong: %+v", res)
	}
}

// Enforce and the group editor write the same list from opposite ends;
// the agent's start-time password enforcement must still find its group
// after the console has renamed it.
func TestEnforceStillMatchesAfterARename(t *testing.T) {
	doc, groups := groupsOf(t, groupsSample)
	groups[0].Name = "Wardens of the Flame"
	SetGroups(doc, groups)

	pw := "enforced"
	if !Enforce(doc, Enforcement{AdminPassword: &pw}) {
		t.Fatal("enforcement reported no change")
	}
	written := GroupsFrom(doc)
	if written[0].Password != pw {
		t.Errorf("enforcement missed the renamed admin group: %+v", written[0])
	}
	if len(written) != 2 {
		t.Errorf("enforcement seeded a duplicate group: %+v", written)
	}
}
