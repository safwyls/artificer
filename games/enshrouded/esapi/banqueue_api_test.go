package esapi_test

import (
	"encoding/json"
	"fmt"
	"github.com/safwyls/sampo/core/api/apitest"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug this covers, from a real deployment on 2026-08-16: a ban added
// while the server was running did not survive the restart. The game keeps
// `bannedAccounts` in memory and writes it back out when it stops, so
// anything the console wrote to that file mid-session was erased on the
// way down.
//
// The fix is to stop competing for the file: while the game holds it, an
// edit is queued, and it is written during the restart — after the stop,
// before the start.

func banFile(t *testing.T, install string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(install, "enshrouded_server.json"))
	if err != nil {
		t.Fatalf("reading the config: %v", err)
	}
	return string(data)
}

func power(t *testing.T, app *apitest.App, id int64, action string, admin []*http.Cookie) {
	t.Helper()
	rec := app.Do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/container/%s", id, action), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", action, rec.Code, rec.Body)
	}
}

func getBans(t *testing.T, app *apitest.App, id int64, admin []*http.Cookie) bansDTO {
	t.Helper()
	rec := app.Do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/bans", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get bans: %d %s", rec.Code, rec.Body)
	}
	var got bansDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return got
}

func putBans(t *testing.T, app *apitest.App, id int64, admin []*http.Cookie, bans []banDTO) bansDTO {
	t.Helper()
	rec := app.Do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/bans", id), map[string]any{"bans": bans}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put bans: %d %s", rec.Code, rec.Body)
	}
	var got bansDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return got
}

func TestBanAddedWhileRunningSurvivesTheRestart(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := supervisorServerWithInstall(t, app)

	// Start it: the supervisor seeds enshrouded_server.json before the
	// first launch, so from here the file exists and the game holds it.
	power(t, app, id, "start", admin)

	got := putBans(t, app, id, admin, []banDTO{{Index: -1, ID: banA}})
	if len(got.Pending) != 1 || got.Pending[0].ID != banA || got.Pending[0].Action != "ban" {
		t.Fatalf("the edit was not queued: %+v", got)
	}
	// Deliberately absent from the file: writing it now would look like a
	// success and be erased at the next stop, with nothing left to retry.
	if strings.Contains(banFile(t, install), banA) {
		t.Fatalf("a ban was written into a file the running game owns:\n%s", banFile(t, install))
	}
	// The list the console shows is the file plus what's queued, so the
	// operator sees the ban they just made rather than an empty list that
	// looks like the save failed.
	if len(got.Bans) != 1 || got.Bans[0].ID != banA {
		t.Errorf("the queued ban should still be shown: %+v", got.Bans)
	}

	// The restart is where it lands — in the gap between the stop and the
	// start, which is the only moment nothing else owns the file.
	power(t, app, id, "restart", admin)
	if !strings.Contains(banFile(t, install), banA) {
		t.Fatalf("the queued ban never reached the config:\n%s", banFile(t, install))
	}

	// And now that the game has loaded it and kept it, the queue retires.
	got = getBans(t, app, id, admin)
	if len(got.Bans) != 1 || got.Bans[0].ID != banA {
		t.Fatalf("bans after the restart = %+v", got.Bans)
	}
	if len(got.Pending) != 0 || len(got.Reverted) != 0 {
		t.Errorf("the queue should be empty once the file agrees: %+v", got)
	}
}

func TestLiftQueuedWhileRunningIsAppliedAtTheRestart(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := supervisorServerWithInstall(t, app)
	power(t, app, id, "start", admin)

	putBans(t, app, id, admin, []banDTO{{Index: -1, ID: banA}, {Index: -1, ID: banB}})
	power(t, app, id, "restart", admin)

	// Lift one of the two while the server is up.
	got := getBans(t, app, id, admin)
	if len(got.Bans) != 2 {
		t.Fatalf("expected both bans in the file: %+v", got.Bans)
	}
	got = putBans(t, app, id, admin, []banDTO{{Index: 0, ID: banA}})
	if len(got.Pending) != 1 || got.Pending[0].Action != "lift" {
		t.Fatalf("the lift was not queued: %+v", got)
	}
	if !strings.Contains(banFile(t, install), banB) {
		t.Error("the lift touched the file the running game owns")
	}

	power(t, app, id, "restart", admin)
	if strings.Contains(banFile(t, install), banB) {
		t.Fatalf("the queued lift never reached the config:\n%s", banFile(t, install))
	}
}

// A ban and a lift of the same account, both before either reached the
// game, never happened — the pair cancels rather than queueing two edits
// that undo each other at apply time.
func TestQueueingTheOppositeEditCancelsThePair(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := supervisorServerWithInstall(t, app)
	power(t, app, id, "start", admin)

	putBans(t, app, id, admin, []banDTO{{Index: -1, ID: banA}})
	got := putBans(t, app, id, admin, nil)
	if len(got.Pending) != 0 {
		t.Fatalf("the pair should have cancelled: %+v", got.Pending)
	}

	power(t, app, id, "restart", admin)
	if strings.Contains(banFile(t, install), banA) {
		t.Fatalf("a cancelled ban was written anyway:\n%s", banFile(t, install))
	}
}

// The diagnosis. If the console wrote a change into the config and the
// game came up without it, the game overwrote it — and this build simply
// does not take its ban list from the file. That is worth saying plainly
// rather than showing the ban quietly missing for a second time.
func TestAnOverwrittenBanIsReportedRatherThanLostAgain(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := supervisorServerWithInstall(t, app)
	power(t, app, id, "start", admin)

	putBans(t, app, id, admin, []banDTO{{Index: -1, ID: banA}})
	power(t, app, id, "restart", admin)
	if !strings.Contains(banFile(t, install), banA) {
		t.Fatal("setup: the queued ban should have been applied")
	}

	// Stand in for the game rewriting the array from its own memory.
	cfg := filepath.Join(install, "enshrouded_server.json")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["bannedAccounts"] = []any{}
	stripped, _ := json.MarshalIndent(doc, "", "    ")
	if err := os.WriteFile(cfg, stripped, 0o644); err != nil {
		t.Fatal(err)
	}

	got := getBans(t, app, id, admin)
	if len(got.Reverted) != 1 || got.Reverted[0].ID != banA {
		t.Fatalf("the overwrite was not reported: %+v", got)
	}
	// And it must not read as merely "still waiting", which would send the
	// operator round the same loop again.
	if len(got.Pending) != 0 {
		t.Errorf("an overwritten edit should not be reported as pending: %+v", got.Pending)
	}
}

// With the server down there is nothing to wait for: the write goes
// straight in.
func TestAnEditMadeWhileStoppedIsWrittenImmediately(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := supervisorServerWithInstall(t, app)
	// Start and stop so the config exists without the game holding it.
	power(t, app, id, "start", admin)
	power(t, app, id, "stop", admin)

	putBans(t, app, id, admin, []banDTO{{Index: -1, ID: banA}})
	if !strings.Contains(banFile(t, install), banA) {
		t.Fatalf("a ban made with the server down was not written:\n%s", banFile(t, install))
	}
}
