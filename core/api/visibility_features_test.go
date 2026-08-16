package api_test

import (
	"net/http"
	"testing"
)

// Carried from palcon's visibility suite (drift ledger: visibility_test
// take-F-merge-P): the admin-bypass and per-switch rules are core policy,
// asserted here against the one feature-gated core endpoint (advisor
// chat, riding the calculators view). The pals/storage/inventory
// specifics return with games/palworld's own tests at its port.

func TestHiddenFeatureRefusesMembersButNotAdmins(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	member := makeMember(t, app, admin)

	rec := app.do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"calculators"},
		"players":        map[string][]string{},
	}, admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save visibility: got %d (body %s)", rec.Code, rec.Body)
	}

	chat := map[string]any{"messages": []map[string]any{{"role": "user", "content": "hi"}}}

	// A member is refused at the gate: the point is that nothing behind
	// the view is even attempted, not that it fails for another reason.
	if rec := app.do(t, "POST", "/api/servers/1/advisor", chat, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member POST advisor: got %d, want 403 (body %s)", rec.Code, rec.Body)
	}

	// The admin gets past the gate and fails on the missing advisor key
	// instead — which is what "admins bypass" has to mean to be worth
	// anything.
	if rec := app.do(t, "POST", "/api/servers/1/advisor", chat, admin); rec.Code == http.StatusForbidden {
		t.Fatal("admin was refused a view they turned off; admins are supposed to bypass")
	}
}

func TestHidingOneFeatureLeavesOthersOn(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	member := makeMember(t, app, admin)

	// Hide an unrelated view; the calculators-gated endpoint must stay
	// open — each switch is its own switch.
	rec := app.do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"map"},
		"players":        map[string][]string{},
	}, admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save visibility: got %d (body %s)", rec.Code, rec.Body)
	}
	chat := map[string]any{"messages": []map[string]any{{"role": "user", "content": "hi"}}}
	if rec := app.do(t, "POST", "/api/servers/1/advisor", chat, member); rec.Code == http.StatusForbidden {
		t.Fatal("hiding the map also hid the calculators view")
	}
}
