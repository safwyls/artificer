package palworld

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPacketRoundTrip(t *testing.T) {
	for _, body := range []string{"", "hello", "Broadcast hello_world", strings.Repeat("x", 4000)} {
		var buf bytes.Buffer
		if err := writePacket(&buf, 7, rconTypeExecCommand, body); err != nil {
			t.Fatalf("write: %v", err)
		}
		id, typ, got, err := readPacket(&buf)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if id != 7 || typ != rconTypeExecCommand || got != body {
			t.Errorf("round-trip: id=%d typ=%d body=%q, want 7 %d %q", id, typ, got, rconTypeExecCommand, body)
		}
	}
}

func TestReadPacketRejectsBadSizes(t *testing.T) {
	for _, size := range []int32{0, 9, 1 << 21} {
		var buf bytes.Buffer
		if err := writePacket(&buf, 1, 0, ""); err != nil {
			t.Fatal(err)
		}
		// Overwrite the size prefix with a bogus value.
		b := buf.Bytes()
		b[0], b[1], b[2], b[3] = byte(size), byte(size>>8), byte(size>>16), byte(size>>24)
		if _, _, _, err := readPacket(bytes.NewReader(b)); err == nil {
			t.Errorf("size %d: want error, got nil", size)
		}
	}
}

// fakeRCON is a loopback server speaking just enough Source RCON for the
// client: auth (with optional pre-auth noise packet, which real Palworld
// servers send), then one command per connection.
type fakeRCON struct {
	ln           net.Listener
	password     string
	response     string
	preAuthNoise bool

	mu       sync.Mutex
	commands []string
	// Close the connection after reading a command instead of replying, the
	// way real Palworld servers answer KickPlayer/BanPlayer on some builds.
	dropAfterCommand bool
}

func (f *fakeRCON) setDropAfterCommand() {
	f.mu.Lock()
	f.dropAfterCommand = true
	f.mu.Unlock()
}

func newFakeRCON(t *testing.T, password, response string, preAuthNoise bool) *fakeRCON {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeRCON{ln: ln, password: password, response: response, preAuthNoise: preAuthNoise}
	t.Cleanup(func() { ln.Close() })
	go f.serve()
	return f
}

func (f *fakeRCON) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeRCON) handle(conn net.Conn) {
	defer conn.Close()
	id, typ, body, err := readPacket(conn)
	if err != nil || typ != rconTypeAuth {
		return
	}
	if body != f.password {
		writePacket(conn, -1, rconTypeAuthResp, "")
		return
	}
	if f.preAuthNoise {
		// Empty SERVERDATA_RESPONSE_VALUE before the auth response.
		writePacket(conn, id, 0, "")
	}
	writePacket(conn, id, rconTypeAuthResp, "")

	cmdID, _, cmd, err := readPacket(conn)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	drop := f.dropAfterCommand
	f.mu.Unlock()
	if drop {
		return // deferred Close drops the connection with no reply
	}
	writePacket(conn, cmdID, 0, f.response)
}

func (f *fakeRCON) client() *RCONClient {
	return &RCONClient{addr: f.ln.Addr().String(), password: f.password, timeout: 2 * time.Second}
}

func TestRCONAuthFailure(t *testing.T) {
	f := newFakeRCON(t, "right-password", "", false)
	c := &RCONClient{addr: f.ln.Addr().String(), password: "wrong", timeout: 2 * time.Second}

	_, err := c.exec(context.Background(), "Info")
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("want auth-failure error, got %v", err)
	}
}

func TestRCONSkipsPreAuthNoise(t *testing.T) {
	f := newFakeRCON(t, "pw", "pong", true)
	out, err := f.client().exec(context.Background(), "ping")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out != "pong" {
		t.Errorf("response = %q, want pong", out)
	}
}

func TestRCONShowPlayersParsing(t *testing.T) {
	// Header row, two normal rows, a short row (skipped), and a blank line.
	response := "name,playeruid,steamid\nSam,12345,7656119\nRen é,67890,7656120\nbroken\n\n"
	f := newFakeRCON(t, "pw", response, false)

	players, err := f.client().Players(context.Background())
	if err != nil {
		t.Fatalf("players: %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("got %d players, want 2: %+v", len(players), players)
	}
	if players[0].Name != "Sam" || players[0].PlayerUID != "12345" || players[0].UserID != "7656119" {
		t.Errorf("first player wrong: %+v", players[0])
	}
	if players[1].Name != "Ren é" {
		t.Errorf("second player wrong: %+v", players[1])
	}
}

func TestRCONInfoParsing(t *testing.T) {
	f := newFakeRCON(t, "pw", "Welcome to Pal Server[v0.5.2.63216] My Cool Server", false)

	info, err := f.client().Info(context.Background())
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.ServerName != "My Cool Server" || info.Version != "0.5.2.63216" || info.Transport != "rcon" {
		t.Errorf("info = %+v", info)
	}
}

func TestRCONKickToleratesDroppedReply(t *testing.T) {
	f := newFakeRCON(t, "pw", "", false)
	f.setDropAfterCommand()

	if err := f.client().Kick(context.Background(), "7656119", "bye"); err != nil {
		t.Errorf("kick with dropped reply: want success, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) != 1 || f.commands[0] != "KickPlayer 7656119" {
		t.Errorf("commands = %v, want [KickPlayer 7656119]", f.commands)
	}
}

func TestRCONBroadcastStillFailsOnDroppedReply(t *testing.T) {
	// Only moderation commands are fire-and-forget; everything else must
	// keep reporting a dropped connection.
	f := newFakeRCON(t, "pw", "", false)
	f.setDropAfterCommand()

	if err := f.client().Broadcast(context.Background(), "hello"); err == nil {
		t.Error("broadcast with dropped reply: want error, got nil")
	}
}

func TestRCONBroadcastUnderscores(t *testing.T) {
	f := newFakeRCON(t, "pw", "", false)
	if err := f.client().Broadcast(context.Background(), "restart in 5"); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.commands) != 1 || f.commands[0] != "Broadcast restart_in_5" {
		t.Errorf("commands = %v, want [Broadcast restart_in_5]", f.commands)
	}
}
