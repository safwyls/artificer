package palworld_test

// The Palworld-specific halves of the visibility suite — the parts that
// need this game's contributed routes and feature set. The game-blind
// halves (admin-only editing, whole-state PUT, one-feature-off) live in
// core/api's own visibility tests.

import (
	"net/http"
	"testing"
)

func TestStorageIsItsOwnSwitch(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	member := makeMember(t, app, admin)

	app.Do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"storage"},
		"players":        map[string][]string{},
	}, admin)

	if rec := app.Do(t, "GET", "/api/servers/1/storage", nil, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member GET storage: got %d, want 403 (body %s)", rec.Code, rec.Body)
	}
	// Asking for world loot must not be a way around the switch.
	if rec := app.Do(t, "GET", "/api/servers/1/storage?world=1", nil, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member GET storage?world=1: got %d, want 403 (body %s)", rec.Code, rec.Body)
	}
	if rec := app.Do(t, "GET", "/api/servers/1/storage", nil, admin); rec.Code == http.StatusForbidden {
		t.Fatal("admin was refused a view they turned off; admins are supposed to bypass")
	}
	// Storage and inventory read the same parse but answer different views,
	// so hiding one must leave the other standing.
	if rec := app.Do(t, "GET", "/api/servers/1/inventory", nil, member); rec.Code == http.StatusForbidden {
		t.Fatal("hiding storage also hid inventory")
	}
}

func TestHidingEveryPalsViewClosesTheSharedEndpoint(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	member := makeMember(t, app, admin)

	// /pals answers three views. Switching one off must not close it...
	app.Do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"pals"},
	}, admin)
	if rec := app.Do(t, "GET", "/api/servers/1/pals", nil, member); rec.Code == http.StatusForbidden {
		t.Fatal("Paldex and Calculators still need /pals")
	}

	// ...but switching off all three must, or the payload would still be a
	// download away from anyone who noticed.
	app.Do(t, "PUT", "/api/servers/1/visibility", map[string]any{
		"hiddenFeatures": []string{"pals", "paldex", "calculators"},
	}, admin)
	if rec := app.Do(t, "GET", "/api/servers/1/pals", nil, member); rec.Code != http.StatusForbidden {
		t.Fatalf("member GET pals with every view off: got %d, want 403", rec.Code)
	}
}
