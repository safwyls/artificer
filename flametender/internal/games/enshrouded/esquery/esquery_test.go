package esquery_test

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/safwyls/flametender/internal/games/enshrouded/esquery"
)

// The tests run against a real UDP socket speaking the wire format by
// hand. A2S is a byte protocol with a handshake, and the failures worth
// catching — a challenge answered with the wrong bytes, a field read at
// the wrong offset — only exist at that level.

type fakeServer struct {
	conn *net.UDPConn
	// reply is asked for each datagram received and returns the body to
	// send back (header and reply-type byte included).
	reply func(req []byte) []byte
	// requests records what the client actually sent.
	requests [][]byte
	done     chan struct{}
}

func newFakeServer(t *testing.T, reply func(req []byte) []byte) *fakeServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeServer{conn: conn, reply: reply, done: make(chan struct{})}
	go s.serve()
	t.Cleanup(func() {
		conn.Close()
		<-s.done
	})
	return s
}

func (s *fakeServer) serve() {
	defer close(s.done)
	buf := make([]byte, 1500)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		req := append([]byte{}, buf[:n]...)
		s.requests = append(s.requests, req)
		if out := s.reply(req); out != nil {
			s.conn.WriteToUDP(out, addr)
		}
	}
}

func (s *fakeServer) addr() string { return s.conn.LocalAddr().String() }

// packet builds a simple-header A2S reply.
func packet(kind byte, body ...[]byte) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, 0xFFFFFFFF)
	out = append(out, kind)
	for _, b := range body {
		out = append(out, b...)
	}
	return out
}

func str(s string) []byte { return append([]byte(s), 0) }

func u16(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func i32(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

func f32(v float32) []byte { return i32(int32(math.Float32bits(v))) }

// infoBody is a well-formed A2S_INFO reply for a 16-slot Enshrouded
// server with 3 players on it.
func infoBody() []byte {
	body := []byte{17} // protocol
	body = append(body, str("Grimwood Bastion")...)
	body = append(body, str("L_World")...)
	body = append(body, str("enshrouded")...)
	body = append(body, str("Enshrouded")...)
	// The 16-bit app-id field, which cannot hold 2278520 and truncates.
	body = append(body, u16(2278520&0xFFFF)...)
	body = append(body, 3, 16, 0) // players, max, bots
	body = append(body, 'd', 'w', 0, 0)
	body = append(body, str("1024233")...)
	// Extra data: port, then the 64-bit GameID carrying the real app id.
	body = append(body, 0x80|0x01)
	body = append(body, u16(15637)...)
	body = append(body, i32(2278520)...)
	body = append(body, i32(0)...)
	return body
}

func TestQueryInfoReadsTheFieldsTheConsoleNeeds(t *testing.T) {
	s := newFakeServer(t, func(req []byte) []byte {
		return packet(0x49, infoBody())
	})

	info, err := esquery.QueryInfo(context.Background(), s.addr())
	if err != nil {
		t.Fatalf("QueryInfo: %v", err)
	}
	if info.Name != "Grimwood Bastion" || info.Map != "L_World" {
		t.Errorf("name/map = %q/%q", info.Name, info.Map)
	}
	// The slot count is the fact nothing else in this console can get:
	// the log never carries it, so a 16 here has to come off the wire.
	if info.Players != 3 || info.MaxPlayers != 16 {
		t.Errorf("players = %d/%d, want 3/16", info.Players, info.MaxPlayers)
	}
	if info.Version != "1024233" {
		t.Errorf("version = %q", info.Version)
	}
	// Read out of the extra-data block, not the 16-bit field that would
	// have truncated it to 50296.
	if info.AppID != 2278520 {
		t.Errorf("appId = %d, want Enshrouded's 2278520", info.AppID)
	}
}

// The 2020 handshake: the server answers the first query with a challenge
// and only replies for real once it comes back.
func TestQueryInfoAnswersTheChallenge(t *testing.T) {
	challenge := []byte{0x11, 0x22, 0x33, 0x44}
	var s *fakeServer
	s = newFakeServer(t, func(req []byte) []byte {
		if len(s.requests) == 1 {
			return packet(0x41, challenge)
		}
		// The re-ask has to carry the challenge on the end of the same
		// payload, or the server would just challenge again.
		tail := req[len(req)-4:]
		if string(tail) != string(challenge) {
			t.Errorf("re-ask carried %x, want the challenge %x", tail, challenge)
		}
		return packet(0x49, infoBody())
	})

	info, err := esquery.QueryInfo(context.Background(), s.addr())
	if err != nil {
		t.Fatalf("QueryInfo: %v", err)
	}
	if info.Name != "Grimwood Bastion" {
		t.Errorf("name = %q", info.Name)
	}
	if len(s.requests) != 2 {
		t.Errorf("sent %d requests, want the query and one re-ask", len(s.requests))
	}
}

// A server that never stops challenging must end as an error rather than
// a loop — this runs on the health path.
func TestQueryGivesUpOnAChallengeLoop(t *testing.T) {
	s := newFakeServer(t, func(req []byte) []byte {
		return packet(0x41, []byte{1, 2, 3, 4})
	})

	if _, err := esquery.QueryInfo(context.Background(), s.addr()); err == nil {
		t.Fatal("a challenge loop should be an error")
	}
	if len(s.requests) > 3 {
		t.Errorf("sent %d requests; the client should stop after one re-ask", len(s.requests))
	}
}

// A server that answers without the optional extra-data block has still
// answered; the app id is simply unknown.
func TestInfoWithoutExtraDataStillParses(t *testing.T) {
	body := []byte{17}
	body = append(body, str("Grimwood Bastion")...)
	body = append(body, str("L_World")...)
	body = append(body, str("enshrouded")...)
	body = append(body, str("Enshrouded")...)
	body = append(body, u16(50296)...)
	body = append(body, 0, 16, 0)
	body = append(body, 'd', 'w', 0, 0)
	body = append(body, str("1024233")...)
	s := newFakeServer(t, func(req []byte) []byte { return packet(0x49, body) })

	info, err := esquery.QueryInfo(context.Background(), s.addr())
	if err != nil {
		t.Fatalf("QueryInfo: %v", err)
	}
	if info.MaxPlayers != 16 || info.Version != "1024233" {
		t.Errorf("info = %+v", info)
	}
	// Zero rather than the truncated 50296: a wrong app id is worse than
	// no app id for the only check it serves.
	if info.AppID != 0 {
		t.Errorf("appId = %d, want 0 when no extra data was sent", info.AppID)
	}
}

func TestQueryPlayersReadsNamesAndDurations(t *testing.T) {
	challenge := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	var s *fakeServer
	s = newFakeServer(t, func(req []byte) []byte {
		if len(s.requests) == 1 {
			// The first ask must carry the 0xFFFFFFFF placeholder.
			if got := req[len(req)-4:]; got[0] != 0xFF || got[3] != 0xFF {
				t.Errorf("first A2S_PLAYER carried %x, want the placeholder", got)
			}
			return packet(0x41, challenge)
		}
		body := []byte{2}
		body = append(body, 0)
		body = append(body, str("Ember")...)
		body = append(body, i32(0)...)
		body = append(body, f32(90)...)
		body = append(body, 1)
		body = append(body, str("Wren")...)
		body = append(body, i32(0)...)
		body = append(body, f32(3600)...)
		return packet(0x44, body)
	})

	players, err := esquery.QueryPlayers(context.Background(), s.addr())
	if err != nil {
		t.Fatalf("QueryPlayers: %v", err)
	}
	if len(players) != 2 || players[0].Name != "Ember" || players[1].Name != "Wren" {
		t.Fatalf("players = %+v", players)
	}
	if players[1].Duration != time.Hour {
		t.Errorf("duration = %v, want 1h", players[1].Duration)
	}
}

// A truncated packet has to fail rather than produce a struct of
// plausible zeros — a server reporting 0/0 players would read as "empty"
// on the dashboard, which is a different claim from "didn't answer".
func TestATruncatedReplyIsAnErrorNotAnEmptyServer(t *testing.T) {
	s := newFakeServer(t, func(req []byte) []byte {
		return packet(0x49, []byte{17}, str("Grimwood Bastion"))
	})

	if _, err := esquery.QueryInfo(context.Background(), s.addr()); err == nil {
		t.Fatal("a truncated A2S_INFO should be an error")
	}
}

func TestASplitReplyIsReportedRatherThanTruncated(t *testing.T) {
	s := newFakeServer(t, func(req []byte) []byte {
		out := make([]byte, 4)
		binary.LittleEndian.PutUint32(out, 0xFFFFFFFE)
		return append(out, 0x49, 0x01)
	})

	_, err := esquery.QueryInfo(context.Background(), s.addr())
	if err == nil {
		t.Fatal("a split reply should be an error")
	}
}

// Nothing listening is the ordinary case for a stopped server, and it has
// to come back promptly as an error rather than hanging the caller.
func TestQueryFailsWhenNothingIsListening(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := esquery.QueryInfo(ctx, addr); err == nil {
		t.Fatal("querying a closed port should fail")
	}
}
