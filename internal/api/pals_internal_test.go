package api

import (
	"encoding/json"
	"testing"
)

// TestPalsPayloadFields pins what /pals and /guilds serve for each player.
//
// This is a privacy boundary, not a style rule. PlayerPals is the extractor's
// struct and these two endpoints used to serialise all of it, so adding
// Inventory and Character to it silently put every player's bags on an
// endpoint the Inventory view's switches don't govern — gated view, honoured
// per-player hides, and the same bytes one fetch away.
//
// A new field here is fine; it just has to be a decision. If this fails,
// ask whether the field belongs on an endpoint three views can read.
func TestPalsPayloadFields(t *testing.T) {
	allowed := map[string]bool{
		"uid": true, "nickname": true, "level": true,
		"party": true, "palbox": true, "base": true, "storage": true,
		"lastOnline": true, "lastX": true, "lastY": true,
		"platform": true, "technologyPoints": true,
		"paldeck": true, "captures": true,
	}

	var probe palsPlayer
	b, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for key := range got {
		if !allowed[key] {
			t.Errorf("/pals and /guilds would now serve %q for every player — "+
				"three views read this payload, so check it belongs there", key)
		}
	}
	for key := range allowed {
		if _, ok := got[key]; !ok {
			t.Errorf("%q disappeared from the pals payload; a view probably needs it", key)
		}
	}
}
