package companion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// vaultWithHistory stands in for reliquary's companion tier: one world
// detail per linked world, in the shape core/api's syncWorldDetail
// writes — including the `accepted` ack the whole tier carries.
func vaultWithHistory(t *testing.T, byWorld map[int64][]map[string]any, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// /api/public/sync/{token}/worlds/{id}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		id := int64(0)
		if len(parts) > 0 {
			fmt.Sscan(parts[len(parts)-1], &id)
		}
		versions, ok := byWorld[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": "no such world"})
			return
		}
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"accepted": true,
			"status": map[string]any{
				"world": map[string]any{"id": id, "name": fmt.Sprintf("World %d", id), "gameTitle": "A Game", "headVersion": 2},
			},
			"versions":  versions,
			"uploaders": map[string]string{"5": "hazel", "6": "rook"},
		})
	}))
}

func version(id int64, kind string, conflict bool, uploader int64, at time.Time) map[string]any {
	return map[string]any{
		"id": id, "kind": kind, "conflict": conflict,
		"bytes": 1024, "uploaderId": uploader, "createdAt": at.UTC().Format(time.RFC3339),
	}
}

func appWithLinks(t *testing.T, url string, worldIDs ...int64) *App {
	t.Helper()
	links := make([]WorldLink, 0, len(worldIDs))
	for _, id := range worldIDs {
		links = append(links, WorldLink{WorldID: id, GameTitle: "A Game", Dir: `C:\saves`})
	}
	return NewApp(Config{ServerURL: url, Token: "tok", Links: links}, filepath.Join(t.TempDir(), "config.json"))
}

// Both tabs are one read. Activity is every version of every linked
// world, newest first; Conflicts is that same list filtered, so the two
// cannot disagree about what happened.
func TestHistoryMergesLinkedWorldsNewestFirst(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	srv := vaultWithHistory(t, map[int64][]map[string]any{
		1: {version(1, "import", false, 5, now.Add(-3*time.Hour)), version(2, "checkin", false, 5, now.Add(-1*time.Hour))},
		2: {version(3, "checkpoint", false, 6, now.Add(-2*time.Hour)), version(4, "checkin", true, 6, now)},
	}, nil)
	defer srv.Close()

	h, err := appWithLinks(t, srv.URL, 1, 2).RefreshHistory()
	if err != nil {
		t.Fatalf("RefreshHistory: %v", err)
	}
	if len(h.Entries) != 4 {
		t.Fatalf("entries = %d, want 4 across both worlds", len(h.Entries))
	}
	for i := 1; i < len(h.Entries); i++ {
		if h.Entries[i].CreatedAt.After(h.Entries[i-1].CreatedAt) {
			t.Fatalf("entries are not newest-first: %v then %v", h.Entries[i-1].CreatedAt, h.Entries[i].CreatedAt)
		}
	}
	first := h.Entries[0]
	if first.VersionID != 4 || first.WorldID != 2 {
		t.Errorf("newest entry = v%d of world %d, want v4 of world 2", first.VersionID, first.WorldID)
	}
	// Uploader names come resolved: a numeric id is not an author.
	if first.Uploader != "rook" {
		t.Errorf("uploader = %q, want rook", first.Uploader)
	}
	// The world names its own head, so a row can say which version is
	// the one a checkout would hand you.
	if !h.Entries[2].Head && !h.Entries[1].Head {
		t.Error("no entry was marked as its world's head")
	}

	conflicts := h.Conflicts()
	if len(conflicts) != 1 || conflicts[0].VersionID != 4 {
		t.Fatalf("conflicts = %+v, want only v4", conflicts)
	}
	// Filtered from the same list, never fetched separately.
	if !conflicts[0].Conflict {
		t.Error("a conflict entry does not carry the flag it was selected by")
	}
}

// A vault that answered for one world and refused for another must say
// so. A short list that looks complete is worse than an error: the whole
// point of this view is noticing something you did not do yourself.
func TestHistoryReportsTheWorldsItCouldNotRead(t *testing.T) {
	now := time.Now()
	srv := vaultWithHistory(t, map[int64][]map[string]any{
		1: {version(1, "checkin", false, 5, now)},
	}, nil)
	defer srv.Close()

	h, err := appWithLinks(t, srv.URL, 1, 99).RefreshHistory()
	if err != nil {
		t.Fatalf("RefreshHistory: %v", err)
	}
	if len(h.Entries) != 1 {
		t.Errorf("entries = %d, want the one world that answered", len(h.Entries))
	}
	if len(h.Failed) != 1 || h.Failed[0].WorldID != 99 {
		t.Fatalf("failed = %+v, want world 99", h.Failed)
	}
	if h.Failed[0].Error == "" {
		t.Error("a failure with no reason is not a report")
	}
}

// Two tabs read this, and a re-render must not re-ask the vault once per
// linked world.
func TestHistoryIsCachedBetweenReads(t *testing.T) {
	var hits int32
	srv := vaultWithHistory(t, map[int64][]map[string]any{
		1: {version(1, "checkin", false, 5, time.Now())},
	}, &hits)
	defer srv.Close()

	a := appWithLinks(t, srv.URL, 1)
	if _, err := a.History(); err != nil {
		t.Fatalf("History: %v", err)
	}
	if _, err := a.History(); err != nil {
		t.Fatalf("History: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("asked the vault %d times for two reads; the cache is not holding", got)
	}
	// The view's own refresh must still reach the vault.
	if _, err := a.RefreshHistory(); err != nil {
		t.Fatalf("RefreshHistory: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("an explicit refresh asked %d times total, want 2", got)
	}
}

// Before a vault is configured there is no history to have, and an empty
// list would read as "nothing has ever happened".
func TestHistoryWithoutAVaultSaysSo(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	if _, err := a.History(); err == nil {
		t.Fatal("an unconfigured companion returned a history instead of a reason")
	}
}

// The page reads these as arrays. A nil slice marshals to null, which is
// the bug this codebase already learned once.
func TestHistoryArraysAreEmptyNotNull(t *testing.T) {
	srv := vaultWithHistory(t, map[int64][]map[string]any{}, nil)
	defer srv.Close()

	h, err := NewApp(Config{ServerURL: srv.URL, Token: "tok"}, filepath.Join(t.TempDir(), "config.json")).RefreshHistory()
	if err != nil {
		t.Fatalf("RefreshHistory: %v", err)
	}
	raw, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("history marshalled a null: %s", raw)
	}
}
