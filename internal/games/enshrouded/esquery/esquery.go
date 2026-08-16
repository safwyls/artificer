// Package esquery speaks the Steam A2S query protocol to a running
// Enshrouded server.
//
// Why this exists at all: Enshrouded has no RCON, no admin API and no
// console, so everything the rest of this repo knows about a live server
// is inferred from its log tail (internal/games/enshrouded/eslog). That
// inference is good but it is inference — it can only know about players
// whose join line is still inside the agent's log ring, and it can never
// know the configured slot count, because the log doesn't carry it.
//
// The one thing the game does answer is the Steam query on its single UDP
// port (docs/enshrouded-recon.md, "Ports & protocols"): `queryPort`
// carries game traffic and A2S both. That gives three facts nothing else
// can: who is present *right now* rather than who was seen to arrive, the
// real `slotCount` behind the server, and the build the game is actually
// running — which is what turns a friend's version-mismatch join error
// from a mystery into a line on the dashboard.
//
// Deliberately not a general A2S library. It implements the two queries
// this console has a use for, against one server at a time, with the
// challenge handshake Valve added in 2020 and no caching — the caller
// decides how often to ask.
package esquery

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)

// Timeout is the default deadline for a whole query, handshake included.
// Short on purpose: this runs on the health path, and a server that
// doesn't answer promptly is reported as not answering rather than
// holding the page.
const Timeout = 3 * time.Second

// Info is the A2S_INFO reply, narrowed to the fields this console uses.
type Info struct {
	// Name is the server-browser name — the game's own copy of the
	// config's `name`, and so the first place a config edit that never
	// reached the game would show up.
	Name string `json:"name"`
	// Map is the world/level name.
	Map string `json:"map"`
	// Players and MaxPlayers are the live count and the configured slot
	// count. MaxPlayers is the number this console cannot get any other
	// way: the log never mentions it and the config on disk is what the
	// game was *asked* for, not what it is running with.
	Players    int `json:"players"`
	MaxPlayers int `json:"maxPlayers"`
	Bots       int `json:"bots"`
	// Version is the game build string as the server reports it.
	Version string `json:"version"`
	// AppID should be Enshrouded's 2278520 — carried so a caller can
	// notice it is pointed at something else entirely.
	//
	// It comes from the extra-data block, not the reply's own app-id
	// field, which is 16 bits and so cannot hold any modern app id:
	// Enshrouded's truncates to 50296 there. A server that sends no extra
	// data leaves this zero rather than the truncated number, because a
	// wrong id is worse than a missing one for the only check it exists
	// to serve.
	AppID int `json:"appId"`
	// Protocol and VAC are reported for completeness; nothing reads them
	// yet.
	Protocol byte `json:"protocol"`
	VAC      bool `json:"vac"`
}

// PlayerEntry is one row of the A2S_PLAYER reply.
//
// Note what is *not* here: an account id. A2S returns names and durations
// only, so this can say how many people are on and how long they have
// been on, but never who they are in a way that could be banned. That is
// why the log tracker stays the roster's source and this stays the
// presence check.
type PlayerEntry struct {
	Name string `json:"name"`
	// Score is carried because the protocol has it; Enshrouded has no
	// notion of one and reports zero.
	Score int32 `json:"score"`
	// Duration is how long the player has been connected.
	Duration time.Duration `json:"-"`
	Seconds  float64       `json:"seconds"`
}

// ErrChallengeLoop is returned when the server keeps answering a query
// with a new challenge instead of data. One re-ask is the protocol; a
// second is a server that isn't going to answer.
var ErrChallengeLoop = errors.New("a2s: server kept issuing challenges instead of replying")

const (
	headerSimple = 0xFFFFFFFF
	headerSplit  = 0xFFFFFFFE

	requestInfo    = 0x54 // 'T'
	requestPlayer  = 0x55 // 'U'
	replyInfo      = 0x49 // 'I'
	replyPlayer    = 0x44 // 'D'
	replyChallenge = 0x41 // 'A'
)

var infoPayload = append([]byte{requestInfo}, []byte("Source Engine Query\x00")...)

// QueryInfo asks a server for A2S_INFO. addr is host:port.
func QueryInfo(ctx context.Context, addr string) (*Info, error) {
	body, err := exchange(ctx, addr, func(challenge []byte) []byte {
		return append(append([]byte{}, infoPayload...), challenge...)
	}, replyInfo)
	if err != nil {
		return nil, err
	}
	return parseInfo(body)
}

// QueryPlayers asks a server for A2S_PLAYER.
func QueryPlayers(ctx context.Context, addr string) ([]PlayerEntry, error) {
	body, err := exchange(ctx, addr, func(challenge []byte) []byte {
		if len(challenge) == 0 {
			// The pre-handshake placeholder the protocol specifies; a
			// server that wants a real challenge answers with one.
			challenge = []byte{0xFF, 0xFF, 0xFF, 0xFF}
		}
		return append([]byte{requestPlayer}, challenge...)
	}, replyPlayer)
	if err != nil {
		return nil, err
	}
	return parsePlayers(body)
}

// exchange sends a query and returns the reply body (everything after the
// reply-type byte), transparently answering the challenge the server may
// issue instead of data.
//
// The challenge is answered exactly once. A server looping on challenges
// is either misconfigured or not the server we think it is, and retrying
// forever on a health path would turn that into a hang.
func exchange(ctx context.Context, addr string, build func(challenge []byte) []byte, want byte) ([]byte, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("a2s dial %s: %w", addr, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	var challenge []byte
	for attempt := 0; attempt < 2; attempt++ {
		if err := send(conn, build(challenge)); err != nil {
			return nil, err
		}
		kind, body, err := receive(conn)
		if err != nil {
			return nil, err
		}
		switch kind {
		case want:
			return body, nil
		case replyChallenge:
			if len(body) < 4 {
				return nil, errors.New("a2s: truncated challenge")
			}
			challenge = body[:4]
		default:
			return nil, fmt.Errorf("a2s: unexpected reply type 0x%02X", kind)
		}
	}
	return nil, ErrChallengeLoop
}

func send(conn net.Conn, payload []byte) error {
	packet := make([]byte, 4, 4+len(payload))
	binary.LittleEndian.PutUint32(packet, headerSimple)
	packet = append(packet, payload...)
	if _, err := conn.Write(packet); err != nil {
		return fmt.Errorf("a2s write: %w", err)
	}
	return nil
}

// receive reads one datagram and splits off the reply-type byte.
//
// Split replies (the 0xFFFFFFFE header) are reported rather than
// reassembled: they need per-engine handling of compression and ordering,
// and an Enshrouded A2S_INFO is a few hundred bytes. Saying so beats
// half-implementing it and returning a truncated name.
func receive(conn net.Conn) (byte, []byte, error) {
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, nil, fmt.Errorf("a2s read: %w", err)
	}
	if n < 5 {
		return 0, nil, errors.New("a2s: reply too short to be one")
	}
	switch binary.LittleEndian.Uint32(buf[:4]) {
	case headerSimple:
	case headerSplit:
		return 0, nil, errors.New("a2s: server sent a split reply, which this client does not reassemble")
	default:
		return 0, nil, errors.New("a2s: reply is not an A2S packet")
	}
	return buf[4], buf[5:n], nil
}

// reader walks a reply body. Every read is bounds-checked and the first
// failure sticks, so a malformed or truncated packet ends as one error
// rather than a struct of plausible-looking zeros.
type reader struct {
	buf []byte
	pos int
	err error
}

func (r *reader) byte() byte {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.buf) {
		r.err = errors.New("a2s: reply ended mid-field")
		return 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b
}

// str reads a NUL-terminated string. The game's own name field is
// operator-supplied, so it is returned as-is rather than validated —
// callers that render it are responsible for escaping, as they are for
// every other name in this console.
func (r *reader) str() string {
	if r.err != nil {
		return ""
	}
	for i := r.pos; i < len(r.buf); i++ {
		if r.buf[i] == 0 {
			s := string(r.buf[r.pos:i])
			r.pos = i + 1
			return s
		}
	}
	r.err = errors.New("a2s: unterminated string in reply")
	return ""
}

func (r *reader) uint16() uint16 {
	if r.err != nil {
		return 0
	}
	if r.pos+2 > len(r.buf) {
		r.err = errors.New("a2s: reply ended mid-field")
		return 0
	}
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v
}

func (r *reader) int32() int32 {
	if r.err != nil {
		return 0
	}
	if r.pos+4 > len(r.buf) {
		r.err = errors.New("a2s: reply ended mid-field")
		return 0
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return int32(v)
}

func (r *reader) float32() float32 {
	return math.Float32frombits(uint32(r.int32()))
}

func parseInfo(body []byte) (*Info, error) {
	r := &reader{buf: body}
	info := &Info{}
	info.Protocol = r.byte()
	info.Name = r.str()
	info.Map = r.str()
	r.str()    // folder
	r.str()    // game
	r.uint16() // the 16-bit app id, useless for modern ids — see Info.AppID
	info.Players = int(r.byte())
	info.MaxPlayers = int(r.byte())
	info.Bots = int(r.byte())
	r.byte() // server type
	r.byte() // environment
	r.byte() // visibility
	info.VAC = r.byte() == 1
	info.Version = r.str()
	if r.err != nil {
		return nil, r.err
	}
	// Everything above is mandatory; everything below is not. A server
	// that stops here has answered the query, so a missing extra-data
	// block is not an error — only a malformed one would be, and even
	// that costs nothing worth failing the whole reply over.
	info.AppID = parseExtraAppID(r)
	return info, nil
}

// Extra-data flags, in the order the block lays them out.
const (
	edfPort      = 0x80
	edfSteamID   = 0x10
	edfSpectator = 0x40
	edfKeywords  = 0x20
	edfGameID    = 0x01
)

// parseExtraAppID walks the optional extra-data block for the 64-bit
// GameID, whose low 24 bits are the real app id. Every earlier field has
// to be stepped over in order to reach it, which is the whole reason this
// block is parsed at all.
func parseExtraAppID(r *reader) int {
	if r.pos >= len(r.buf) {
		return 0
	}
	edf := r.byte()
	if edf&edfPort != 0 {
		r.uint16()
	}
	if edf&edfSteamID != 0 {
		r.int32()
		r.int32()
	}
	if edf&edfSpectator != 0 {
		r.uint16()
		r.str()
	}
	if edf&edfKeywords != 0 {
		r.str()
	}
	if edf&edfGameID == 0 {
		return 0
	}
	low := uint32(r.int32())
	r.int32() // the high half, which no app id reaches
	if r.err != nil {
		// A short or malformed tail says nothing about the fields already
		// read, so it is swallowed here and reported as "no app id".
		r.err = nil
		return 0
	}
	return int(low & 0xFFFFFF)
}

func parsePlayers(body []byte) ([]PlayerEntry, error) {
	r := &reader{buf: body}
	count := int(r.byte())
	if r.err != nil {
		return nil, r.err
	}
	out := make([]PlayerEntry, 0, count)
	for i := 0; i < count; i++ {
		r.byte() // index, which the protocol documents as unreliable
		name := r.str()
		score := r.int32()
		secs := r.float32()
		if r.err != nil {
			// A reply that runs out mid-list still described the players it
			// did carry, and a short roster beats no roster on a presence
			// check. The count is what the server said, not what parsed.
			break
		}
		out = append(out, PlayerEntry{
			Name:     name,
			Score:    score,
			Duration: time.Duration(float64(secs) * float64(time.Second)),
			Seconds:  float64(secs),
		})
	}
	if len(out) == 0 && r.err != nil {
		return nil, r.err
	}
	return out, nil
}
