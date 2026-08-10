package wkagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeHeartbeat drops a heartbeat.json the way the mod would.
func writeHeartbeat(t *testing.T, dir string, ageSeconds int, commands ...string) {
	t.Helper()
	hb := bridgeHeartbeat{
		TS:       time.Now().Add(-time.Duration(ageSeconds) * time.Second).Unix(),
		Version:  "dwbridge/test",
		Protocol: 1,
		Commands: commands,
	}
	data, _ := json.Marshal(hb)
	if err := os.WriteFile(filepath.Join(dir, "heartbeat.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestBridge(t *testing.T) *bridge {
	t.Helper()
	dir := t.TempDir()
	b := newBridge(dir)
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBridgeStatusNoDir(t *testing.T) {
	// A server with no mod at all: no dwbridge directory. Status must be nil
	// (not "unavailable"), so the control plane can tell "no channel exists"
	// from "one exists but is down".
	b := newBridge(t.TempDir()) // dir not created
	if s := b.Status(); s != nil {
		t.Fatalf("Status() = %+v, want nil when no bridge dir exists", s)
	}
}

func TestBridgeStatusFreshAndStale(t *testing.T) {
	b := newTestBridge(t)

	writeHeartbeat(t, b.dir, 1, "ping", "save")
	s := b.Status()
	if s == nil || !s.Available {
		t.Fatalf("fresh heartbeat: Status() = %+v, want available", s)
	}
	if len(s.Commands) != 2 || s.Commands[0] != "ping" {
		t.Errorf("commands = %v", s.Commands)
	}

	writeHeartbeat(t, b.dir, 60, "ping")
	s = b.Status()
	if s == nil || s.Available {
		t.Fatalf("stale heartbeat: Status() = %+v, want present but unavailable", s)
	}
}

// startFakeMod simulates the UE4SS mod: it polls request.json, claims it by
// deleting it, runs a handler, and writes response.json echoing the id —
// exactly the single-flight protocol the real mod follows.
func startFakeMod(t *testing.T, b *bridge, handler func(command string, args map[string]string) (bool, string, any)) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	reqPath := filepath.Join(b.dir, requestFile)
	respPath := filepath.Join(b.dir, responseFile)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if data, err := os.ReadFile(reqPath); err == nil {
				os.Remove(reqPath) // claim before running
				var req struct {
					ID      string            `json:"id"`
					Command string            `json:"command"`
					Args    map[string]string `json:"args"`
				}
				if json.Unmarshal(data, &req) == nil {
					ok, errMsg, payload := handler(req.Command, req.Args)
					resp := map[string]any{"id": req.ID, "ok": ok}
					if errMsg != "" {
						resp["error"] = errMsg
					}
					if payload != nil {
						resp["data"] = payload
					}
					body, _ := json.Marshal(resp)
					_ = writeFileAtomic(respPath, body)
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

func TestBridgeCommandRoundTrip(t *testing.T) {
	b := newTestBridge(t)
	writeHeartbeat(t, b.dir, 0, "ping", "save")
	var gotCommand string
	var gotArgs map[string]string
	startFakeMod(t, b, func(command string, args map[string]string) (bool, string, any) {
		gotCommand, gotArgs = command, args
		return true, "", map[string]any{"world": true}
	})

	data, err := b.Command(context.Background(), "save", map[string]string{"slot": "main"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if gotCommand != "save" || gotArgs["slot"] != "main" {
		t.Errorf("mod saw command=%q args=%v", gotCommand, gotArgs)
	}
	var payload struct {
		World bool `json:"world"`
	}
	if json.Unmarshal(data, &payload) != nil || !payload.World {
		t.Errorf("data = %s, want {world:true}", data)
	}

	// The request file must be gone — neither side should leave litter.
	if _, err := os.Stat(filepath.Join(b.dir, requestFile)); !os.IsNotExist(err) {
		t.Errorf("request.json left behind after a completed command")
	}
}

func TestBridgeCommandModError(t *testing.T) {
	b := newTestBridge(t)
	writeHeartbeat(t, b.dir, 0, "save")
	startFakeMod(t, b, func(string, map[string]string) (bool, string, any) {
		return false, "no world loaded", nil
	})
	_, err := b.Command(context.Background(), "save", nil)
	if err == nil || errors.Is(err, errBridgeUnavailable) {
		t.Fatalf("err = %v, want a plain handler error (not unavailable)", err)
	}
}

func TestBridgeCommandUnknownVerb(t *testing.T) {
	b := newTestBridge(t)
	writeHeartbeat(t, b.dir, 0, "save") // mod does not offer "kick"
	_, err := b.Command(context.Background(), "kick", nil)
	if err == nil {
		t.Fatal("expected error for a command the mod doesn't implement")
	}
	if errors.Is(err, errBridgeUnavailable) {
		t.Errorf("err = %v, want a capability error, not unavailable", err)
	}
}

func TestBridgeCommandUnavailable(t *testing.T) {
	b := newTestBridge(t)
	writeHeartbeat(t, b.dir, 60, "save") // stale
	_, err := b.Command(context.Background(), "save", nil)
	if !errors.Is(err, errBridgeUnavailable) {
		t.Fatalf("err = %v, want errBridgeUnavailable for a stale heartbeat", err)
	}
}

func TestBridgeCommandTimeoutCleansUp(t *testing.T) {
	b := newTestBridge(t)
	writeHeartbeat(t, b.dir, 0, "save")
	// No mod running: the request is never answered.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := b.Command(ctx, "save", nil)
	if !errors.Is(err, errBridgeUnavailable) {
		t.Fatalf("err = %v, want errBridgeUnavailable on timeout", err)
	}
	// The orphaned .req must be cleaned up so the dir doesn't fill.
	entries, _ := os.ReadDir(b.dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".req" {
			t.Errorf("timed-out request left behind: %s", e.Name())
		}
	}
}
