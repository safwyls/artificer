package savedb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A faithful excerpt of the real manifest: quoted and unquoted keys, a
// game with no save locations at all, save and config entries side by
// side, per-store variants, extra Steam ids, and an install dir that
// differs from the title.
const fixture = `---
"!4RC4N01D!":
  steam:
    id: 777010
Palworld:
  cloud:
    steam: true
  files:
    "<winLocalAppData>/Packages/PocketpairInc.Palworld_ad4psfrxyesvt/SystemAppData/wgs/<storeUserId>":
      tags:
        - save
      when:
        - os: windows
          store: microsoft
    "<winLocalAppData>/Pal/Saved/Config/Windows":
      tags:
        - config
      when:
        - os: windows
    "<winLocalAppData>/Pal/Saved/SaveGames/<storeUserId>":
      tags:
        - save
      when:
        - os: windows
  id:
    steamExtra:
      - 2771110
  installDir:
    Palworld: {}
  steam:
    id: 1623730
"Baldur's Gate 3":
  files:
    "<winLocalAppData>/Larian Studios/Baldur's Gate 3/PlayerProfiles/Public/Savegames/Story":
      tags:
        - save
      when:
        - os: windows
  installDir:
    "Baldurs Gate 3": {}
  steam:
    id: 1086940
"Config Only Game":
  files:
    "<winAppData>/ConfigOnly":
      tags:
        - config
  steam:
    id: 999999
"Anywhere Game":
  files:
    "<home>/.anywhere":
      tags:
        - save
  steam:
    id: 424242
`

func loaded(t *testing.T, body string) (*Client, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return c, &hits
}

func TestLookup(t *testing.T) {
	c, _ := loaded(t, fixture)

	if st := c.Status(); !st.Loaded || st.Games != 3 {
		// !4RC4N01D! and Config Only Game carry no save locations.
		t.Errorf("status = %+v, want 3 games with saves", st)
	}

	// By Steam app id: the exact path, and both store variants come back
	// so the companion can decide which apply to it.
	got := c.Lookup([]Query{{AppID: "1623730", Name: "Palworld"}})["app:1623730"]
	if len(got) != 2 {
		t.Fatalf("Palworld locations = %+v, want the two save entries (not the config one)", got)
	}
	var templates []string
	for _, l := range got {
		templates = append(templates, l.Template)
	}
	joined := strings.Join(templates, " ")
	if !strings.Contains(joined, "<winLocalAppData>/Pal/Saved/SaveGames/<storeUserId>") {
		t.Errorf("locations = %v, want the Steam save path", templates)
	}
	if strings.Contains(joined, "Saved/Config") {
		t.Error("a config folder came back as a save location")
	}

	// An extra Steam id resolves to the same game — a re-release or a
	// demo shares the save folder.
	if len(c.Lookup([]Query{{AppID: "2771110"}})["app:2771110"]) != 2 {
		t.Error("steamExtra id did not resolve")
	}

	// The install folder is the manifest's own name for what Steam calls
	// the game, so it beats a title search when the two differ.
	byDir := c.Lookup([]Query{{Name: "unknown title", InstallDir: "Baldurs Gate 3"}})
	if len(byDir["name:unknown title"]) != 1 {
		t.Errorf("install-dir lookup = %+v, want Baldur's Gate 3", byDir)
	}
	// Punctuation and case fold away on the title path.
	if len(c.Lookup([]Query{{Name: "baldurs gate 3"}})["name:baldurs gate 3"]) != 1 {
		t.Error("a title differing only in punctuation did not match")
	}

	// A game with no save locations is absent, not empty — the same
	// answer as a game the manifest has never heard of.
	for _, q := range []Query{{AppID: "999999"}, {AppID: "777010"}, {Name: "no such game"}} {
		if got := c.Lookup([]Query{q}); len(got) != 0 {
			t.Errorf("%+v answered %v, want nothing", q, got)
		}
	}

	// An entry with no `when` applies everywhere rather than nowhere.
	any := c.Lookup([]Query{{AppID: "424242"}})["app:424242"]
	if len(any) != 1 || any[0].OS != "" || any[0].Store != "" {
		t.Errorf("unconstrained entry = %+v, want one location with no constraints", any)
	}
}

// A catalogue that fails to reload keeps the one it had: stale beats
// nothing, and the failure is reported rather than swallowed.
func TestRefreshFailureKeepsTheOldCatalogue(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, fixture)
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	fail = true
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("a failing refresh reported success")
	}
	st := c.Status()
	if !st.Loaded || st.Games != 3 {
		t.Errorf("status = %+v; a failed refresh must keep what was loaded", st)
	}
	if st.LastError == "" {
		t.Error("a failed refresh left no explanation")
	}
	if len(c.Lookup([]Query{{AppID: "1623730"}})) != 1 {
		t.Error("lookups stopped working after a failed refresh")
	}

	// Recovering clears the fault.
	fail = false
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("recovery refresh: %v", err)
	}
	if st := c.Status(); st.LastError != "" {
		t.Errorf("lastError = %q after a good refresh, want it cleared", st.LastError)
	}
}

// Nonsense is refused rather than installed as an empty catalogue that
// silently answers nothing forever.
func TestGarbageIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>not a manifest</html>")
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("a non-manifest body was accepted")
	}
	if c.Loaded() {
		t.Error("client reports loaded after refusing the body")
	}
}

// A nil client is usable and answers nothing.
func TestNilClient(t *testing.T) {
	var c *Client
	if c.Loaded() || len(c.Lookup([]Query{{AppID: "1"}})) != 0 {
		t.Error("a nil client must answer nothing rather than panic")
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Errorf("nil refresh: %v", err)
	}
	c.Run(context.Background())
	if st := c.Status(); st.Loaded {
		t.Error("nil status reports loaded")
	}
}
