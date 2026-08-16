package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The create response is what the client renders next, so it has to match
// the row that was actually written. Normalization used to happen only on
// the way into the database, which left the response claiming an empty
// game for every server added through the form.
func TestCreateServerResponseMatchesStoredRow(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	rec := app.do(t, "POST", "/api/servers", map[string]any{
		"name": "Keep", "host": "10.0.0.9", "enabled": true,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		ID       int64    `json:"id"`
		Game     string   `json:"game"`
		GamePort int      `json:"gamePort"`
		Features []string `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Game != "enshrouded" {
		t.Errorf("create response game = %q, want the normalized id", created.Game)
	}
	if created.GamePort != 15637 {
		t.Errorf("create response gamePort = %d, want the game default", created.GamePort)
	}
	// And the same values must come back on a re-read.
	rec = app.do(t, "GET", "/api/servers/"+itoa(created.ID), nil, admin)
	var fetched struct {
		Game     string `json:"game"`
		GamePort int    `json:"gamePort"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.Game != created.Game || fetched.GamePort != created.GamePort {
		t.Errorf("create response %+v disagrees with stored row %+v", created, fetched)
	}
}
