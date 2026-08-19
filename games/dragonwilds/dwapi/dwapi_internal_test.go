package dwapi

import (
	"testing"

	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

// TestWithCharNames pins the overlay rules: log-learned names land on
// name-less transform records, records that carry their own name keep it,
// and the cached world is never mutated — the overlay works on a copy.
func TestWithCharNames(t *testing.T) {
	world := &dwsave.World{
		WorldName: "grimwood_bastion",
		Players: []dwsave.PlayerCharacter{
			{CharGuid: "044F259443215BB8B575B6ACAA2A1D8D"},
			{CharGuid: "1AD58DFC4130CB0EDF3B21B92DE5F720", CharName: "FromRecord"},
			{CharGuid: "384D68C0479A97B5E99446BAB5A9405D"},
		},
	}
	srv := &store.Server{AgentURL: "http://10.0.0.9:8800"}

	h := &handlers{charNames: func(agentURL string) map[string]string {
		if agentURL != srv.AgentURL {
			t.Errorf("looked up %q", agentURL)
		}
		return map[string]string{
			"044F259443215BB8B575B6ACAA2A1D8D": "Aldra",
			"1AD58DFC4130CB0EDF3B21B92DE5F720": "MustNotOverride",
		}
	}}

	got := h.withCharNames(srv, world)
	if got.Players[0].CharName != "Aldra" {
		t.Errorf("Players[0] = %+v, want the log-learned name", got.Players[0])
	}
	if got.Players[1].CharName != "FromRecord" {
		t.Errorf("Players[1] = %+v, want the record's own name kept", got.Players[1])
	}
	if got.Players[2].CharName != "" {
		t.Errorf("Players[2] = %+v, want no invented name", got.Players[2])
	}
	// The shared cached world must be untouched.
	if world.Players[0].CharName != "" {
		t.Error("overlay mutated the cached world")
	}

	// No lookup wired: the world passes through as-is.
	h2 := &handlers{}
	if h2.withCharNames(srv, world) != world {
		t.Error("nil lookup should pass the cached world through")
	}
}
