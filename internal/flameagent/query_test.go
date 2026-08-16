package flameagent_test

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/flametender/internal/flameagent"
)

// The agent runs the Steam query from inside the container against
// 127.0.0.1, so these tests stand a real UDP responder on a real port and
// point the agent's game port at it. What is being checked is the wiring
// — that the handler asks the port the game was told to bind, and that a
// silent server reads as "no answer" rather than as an agent fault.

// a2sResponder answers A2S_INFO on an ephemeral UDP port.
func a2sResponder(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	body := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49, 17}
	body = append(body, append([]byte("Grimwood Bastion"), 0)...)
	body = append(body, append([]byte("L_World"), 0)...)
	body = append(body, append([]byte("enshrouded"), 0)...)
	body = append(body, append([]byte("Enshrouded"), 0)...)
	appid := make([]byte, 2)
	binary.LittleEndian.PutUint16(appid, uint16(2278520&0xFFFF))
	body = append(body, appid...)
	body = append(body, 2, 16, 0, 'd', 'w', 0, 0)
	body = append(body, append([]byte("1024233"), 0)...)

	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n > 4 && buf[4] == 0x54 {
				conn.WriteToUDP(body, addr)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

// agentOnPort builds a supervisor agent whose game port is the given one,
// so the query handler is aimed at the responder above.
func agentOnPort(t *testing.T, port int) *httptest.Server {
	t.Helper()
	install := t.TempDir()
	writeGame(t, install, "trap 'exit 0' INT TERM\nwhile true; do sleep 0.05; done")
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := flameagent.New(flameagent.Config{
		Token: testToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Mode: "supervisor", GameCommand: "./game.sh", GamePort: port,
		StopGrace: 500 * time.Millisecond,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	return srv
}

func TestQueryEndpointReturnsTheGamesOwnAnswer(t *testing.T) {
	srv := agentOnPort(t, a2sResponder(t))
	do(t, srv, "POST", "/v1/power/start", testToken, nil)

	resp, body := do(t, srv, "GET", "/v1/query", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query: got %d, body %v", resp.StatusCode, body)
	}
	info, ok := body["info"].(map[string]any)
	if !ok {
		t.Fatalf("no info in %v", body)
	}
	if info["name"] != "Grimwood Bastion" {
		t.Errorf("name = %v", info["name"])
	}
	// The slot count is the whole reason the console asks: it is the one
	// number the log stream cannot carry.
	if info["maxPlayers"] != float64(16) || info["players"] != float64(2) {
		t.Errorf("players = %v/%v, want 2/16", info["players"], info["maxPlayers"])
	}
}

// A booting server binds nothing and answers nothing. That is an ordinary
// state, not a fault, and it has to be distinguishable from "the agent
// broke" — so 503 with a reason, never 500.
func TestQueryReportsASilentServerAsUnavailable(t *testing.T) {
	// A port with nothing on it: the closest thing to a game that has not
	// finished coming up.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	srv := agentOnPort(t, port)
	do(t, srv, "POST", "/v1/power/start", testToken, nil)

	resp, body := do(t, srv, "GET", "/v1/query", testToken, nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (body %v)", resp.StatusCode, body)
	}
	if body["error"] == nil {
		t.Error("a 503 should say why")
	}
}

// Querying a stopped game is answered by the agent itself rather than by
// a three-second timeout against a port nobody is on.
func TestQueryOnAStoppedGameAnswersImmediately(t *testing.T) {
	srv := agentOnPort(t, a2sResponder(t))

	start := time.Now()
	resp, body := do(t, srv, "GET", "/v1/query", testToken, nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (body %v)", resp.StatusCode, body)
	}
	if time.Since(start) > time.Second {
		t.Error("a stopped game should not be answered by waiting for a UDP timeout")
	}
}
