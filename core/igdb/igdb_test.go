package igdb_test

// The artwork client against a stand-in for Twitch and IGDB.
//
// The behaviour under test is the one that bit in the field: covers
// simply did not appear, and because every failure degraded to "no
// cover", nothing said why. Two causes are covered here — a filter
// spelling IGDB no longer accepts, and a game whose Steam id IGDB has no
// external record of — plus the diagnostics that now name a third
// (credentials that don't work).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/safwyls/artificer/core/igdb"
)

type fakeIGDB struct {
	*httptest.Server
	mu sync.Mutex
	// acceptFilter is the only external_games filter this stand-in
	// understands; anything else is a 400, the way IGDB answers a field
	// it doesn't have.
	acceptFilter string
	tokenStatus  int
	queries      []string
}

func newFakeIGDB(t *testing.T, acceptFilter string) *fakeIGDB {
	t.Helper()
	f := &fakeIGDB{acceptFilter: acceptFilter, tokenStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		status := f.tokenStatus
		f.mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			io.WriteString(w, `{"status":401,"message":"invalid client secret"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/v4/external_games", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.record(string(body))
		f.mu.Lock()
		accept := f.acceptFilter
		f.mu.Unlock()
		if !strings.Contains(string(body), accept) {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `Invalid Field`)
			return
		}
		// Only app 111 has an external record; 222 is the game IGDB knows
		// by name but not by Steam id.
		out := []map[string]any{}
		if strings.Contains(string(body), `"111"`) {
			out = append(out, map[string]any{"uid": "111", "game": map[string]any{
				"name": "Palworld", "slug": "palworld", "summary": "pals",
				"cover": map[string]any{"image_id": "co1"},
			}})
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/v4/games", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.record(string(body))
		if strings.Contains(string(body), "Enshrouded") {
			json.NewEncoder(w).Encode([]map[string]any{{
				"name": "Enshrouded", "slug": "enshrouded",
				"cover": map[string]any{"image_id": "co2"},
			}})
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1}})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeIGDB) record(body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, body)
}

func (f *fakeIGDB) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

func newClient(t *testing.T, f *fakeIGDB) *igdb.Client {
	t.Helper()
	c := igdb.New("client", "secret")
	c.UseEndpoints(f.URL+"/oauth2/token", f.URL+"/v4")
	return c
}

// A deployment whose IGDB no longer accepts the older `category` filter
// still gets covers: the client tries the other spelling rather than
// treating a rejected query as "this game doesn't exist".
func TestAppIDFilterFallback(t *testing.T) {
	f := newFakeIGDB(t, "external_game_source = 1")
	c := newClient(t, f)

	art := c.Lookup(context.Background(), []igdb.Query{{AppID: "111", Name: "Palworld"}})
	got, ok := art["app:111"]
	if !ok {
		t.Fatalf("no art for app 111; the client asked: %q", f.asked())
	}
	if got.Name != "Palworld" || !strings.Contains(got.Cover, "co1") {
		t.Errorf("art = %+v, want Palworld with a cover URL", got)
	}
	if st := c.Status(); st.Filter != "external_game_source = 1" {
		t.Errorf("learned filter = %q, want the one the server accepted", st.Filter)
	}
	// A rejected shape is not an error the admin should still be staring
	// at once the other one worked.
	if st := c.Status(); st.LastError != "" {
		t.Errorf("lastError = %q after a successful lookup, want it cleared", st.LastError)
	}

	// The learned shape is used first next time: one request, not two.
	before := len(f.asked())
	c.Lookup(context.Background(), []igdb.Query{{AppID: "999", Name: ""}})
	if n := len(f.asked()) - before; n != 1 {
		t.Errorf("second batch made %d external_games requests, want 1 now the shape is known", n)
	}
}

// The same works the other way round, so a deployment on the older API
// is not broken by preferring the newer spelling.
func TestAppIDFilterFallbackReversed(t *testing.T) {
	f := newFakeIGDB(t, "category = 1")
	c := newClient(t, f)
	if _, ok := c.Lookup(context.Background(), []igdb.Query{{AppID: "111"}})["app:111"]; !ok {
		t.Fatalf("no art for app 111 on the older filter; asked: %q", f.asked())
	}
}

// A game IGDB carries but has no Steam external record for still gets a
// cover, from its name — the case that made a whole shelf look empty
// when only the id lookup was tried.
func TestNameFallbackForUnknownAppID(t *testing.T) {
	f := newFakeIGDB(t, "external_game_source = 1")
	c := newClient(t, f)

	art := c.Lookup(context.Background(), []igdb.Query{{AppID: "222", Name: "Enshrouded"}})
	got, ok := art["app:222"]
	if !ok {
		t.Fatalf("no art for app 222; asked: %q", f.asked())
	}
	if got.Name != "Enshrouded" || !strings.Contains(got.Cover, "co2") {
		t.Errorf("art = %+v, want the name-search result under the app-id key", got)
	}
}

// Credentials that don't work are the failure the first cut hid
// completely: the lookup still degrades to no covers, but Status names
// the cause in IGDB's own words.
func TestBadCredentialsAreVisible(t *testing.T) {
	f := newFakeIGDB(t, "external_game_source = 1")
	f.mu.Lock()
	f.tokenStatus = http.StatusUnauthorized
	f.mu.Unlock()
	c := newClient(t, f)

	if art := c.Lookup(context.Background(), []igdb.Query{{AppID: "111"}}); len(art) != 0 {
		t.Errorf("art = %v with a rejected credential, want none", art)
	}
	st := c.Status()
	if !st.Configured {
		t.Error("status reports unconfigured, but both halves of the credential are set")
	}
	if !strings.Contains(st.LastError, "invalid client secret") {
		t.Errorf("lastError = %q, want IGDB's own words about the credential", st.LastError)
	}
	if err := c.Test(context.Background()); err == nil {
		t.Error("Test passed with a rejected credential")
	}

	// Fixing the credential clears the fault and re-tries what missed —
	// a saved-but-stale miss would otherwise keep the shelf blank for
	// hours after the fix.
	f.mu.Lock()
	f.tokenStatus = http.StatusOK
	f.mu.Unlock()
	c.SetCredentials("client", "better-secret", "settings")
	if _, ok := c.Lookup(context.Background(), []igdb.Query{{AppID: "111"}})["app:111"]; !ok {
		t.Fatalf("no art after fixing the credential; asked: %q", f.asked())
	}
	if st := c.Status(); st.LastError != "" || st.Source != "settings" {
		t.Errorf("status = %+v, want no error and the settings source", st)
	}
}

// An unconfigured client is not an error anywhere: it answers nothing,
// and says so.
func TestUnconfiguredIsQuiet(t *testing.T) {
	c := igdb.New("", "")
	if c == nil {
		t.Fatal("New returned nil; an unconfigured client must still be usable")
	}
	if c.Configured() {
		t.Error("Configured() is true with no credentials")
	}
	if art := c.Lookup(context.Background(), []igdb.Query{{Name: "Palworld"}}); len(art) != 0 {
		t.Errorf("art = %v with no credentials, want none", art)
	}
	if err := c.Test(context.Background()); err == nil {
		t.Error("Test passed with no credentials configured")
	}
	var nilClient *igdb.Client
	if nilClient.Configured() || len(nilClient.Lookup(context.Background(), []igdb.Query{{Name: "x"}})) != 0 {
		t.Error("a nil client must answer nothing rather than panic")
	}
}
