package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func charFile(t *testing.T, dir, name, charName string) string {
	t.Helper()
	rec := map[string]any{
		"meta_data": map[string]any{
			"char_guid": "ESIzRFVmd4iZqrvM3e7_AA",
			"char_name": charName,
		},
		"SaveCount": 3,
		"Skills": map[string]any{
			"Skills": []any{map[string]any{"Id": "4zYUGF5u_0KbMLkWJmmBbQ", "Xp": 388}},
		},
	}
	b, _ := json.Marshal(rec)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestScanAndRelay drives the loop's pieces: the scan finds character
// records and ignores everything else, pushes go out once per change plus
// on the forced heartbeat, and the console's acknowledgment lands in the
// relay status.
func TestScanAndRelay(t *testing.T) {
	dir := t.TempDir()
	path := charFile(t, dir, "Aldra.sav", "Aldra")
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "junk.sav"), []byte("SAVEnotjson"), 0o644)

	var pushes atomic.Int32
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			pushes.Add(1)
		}
		json.NewEncoder(w).Encode(map[string]any{"accepted": true, "server": "grimwood"})
	}))
	defer console.Close()

	a := newApp(Config{ConsoleURL: console.URL, Token: "tok", SaveDir: dir}, filepath.Join(t.TempDir(), "cfg.json"))
	a.scan()

	a.mu.Lock()
	n := len(a.characters)
	a.mu.Unlock()
	if n != 1 {
		t.Fatalf("characters = %d, want just the real record", n)
	}

	if !a.pushChanged(false) {
		t.Fatal("push failed")
	}
	if got := pushes.Load(); got != 1 {
		t.Fatalf("pushes = %d, want 1", got)
	}
	// Unchanged: nothing due.
	a.pushChanged(false)
	if got := pushes.Load(); got != 1 {
		t.Fatalf("pushes after no change = %d, want still 1", got)
	}
	// The heartbeat re-pushes regardless.
	a.pushChanged(true)
	if got := pushes.Load(); got != 2 {
		t.Fatalf("pushes after heartbeat = %d, want 2", got)
	}
	// A changed file is due again.
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(path, future, future)
	a.scan()
	a.pushChanged(false)
	if got := pushes.Load(); got != 3 {
		t.Fatalf("pushes after file change = %d, want 3", got)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.relay.Server != "grimwood" || a.relay.LastPushAt == nil || a.relay.LastError != "" {
		t.Errorf("relay = %+v", a.relay)
	}
}

// TestRelayError keeps a refusing console visible: the error lands in the
// status instead of vanishing, and the record stays due for retry.
func TestRelayError(t *testing.T) {
	dir := t.TempDir()
	charFile(t, dir, "Aldra.sav", "Aldra")
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer console.Close()

	a := newApp(Config{ConsoleURL: console.URL, Token: "revoked", SaveDir: dir}, filepath.Join(t.TempDir(), "cfg.json"))
	a.scan()
	if a.pushChanged(false) {
		t.Fatal("push against a 404 console reported success")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.relay.LastError == "" {
		t.Error("LastError empty after a refused push")
	}
	for _, c := range a.characters {
		if c.PushedAt != nil {
			t.Error("a refused record must stay due for retry")
		}
	}
}

func TestNormalizeConsoleURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://x.example.com/":    "https://x.example.com",
		"https://x.example.com/api": "https://x.example.com",
		" https://x.example.com ":   "https://x.example.com",
	} {
		if got := normalizeConsoleURL(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
