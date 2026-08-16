// Package enshrouded implements flametender's game contract for Enshrouded
// dedicated servers.
//
// The game has no RCON, no HTTP admin API and no server console — all
// native administration is enshrouded_server.json (edit + restart) plus
// the in-game player menu for whoever holds a kick/ban-capable role
// password. Everything flametender shows is therefore *derived*: process
// liveness from the flameagent sidecar's health, the player list from a
// state machine over the agent's log tail (eslog), config from
// enshrouded_server.json at rest (esconfig). The one native query surface
// the game does have — Steam A2S on its UDP port — is roadmap Phase 2;
// the log-derived roster stands alone until then.
//
// docs/enshrouded-recon.md is the source for every external fact used
// here, and marks which of them are still unverified against a real
// server.
package enshrouded

import (
	"strings"

	"github.com/safwyls/flametender/internal/game"
)

// AppID is the Steam app id of the *dedicated server* tool (the game
// client is 1203620). Deliberately duplicated as flameagent.DefaultAppID
// the same way dragonwilds did it in wildskeeper — the agent is a thin
// sidecar that must not link the game registry — and an agreement test
// keeps the two honest.
const AppID = 2278520

// MaxSlots is the game's hard cap on slotCount. The configured slotCount
// can be anything from 1 to this; nothing derived from logs reports it,
// so charts use the cap until the A2S query (Phase 2) can report the
// server's own number.
const MaxSlots = 16

// DefaultQueryPort is Enshrouded's single UDP port — game traffic and the
// Steam A2S query share it. There has been no separate gamePort since
// Melodies of the Mire (2024-06); recon doc, "Ports & protocols".
const DefaultQueryPort = 15637

// Definition registers Enshrouded with the shared layer. The feature list
// is deliberately small and honest: players (the Flameborn view), world
// saves, and the live log tail. No map and no inventories — the world
// save is an unparsed proprietary blob, and player characters never touch
// the server at all (they live client-side).
var Definition = &game.Definition{
	ID:              "enshrouded",
	Name:            "Enshrouded",
	DefaultGamePort: DefaultQueryPort,
	NewClient:       New,
	CanonicalUID:    CanonicalUID,
	Features: []string{
		game.FeaturePals,
		game.FeatureSaves,
		game.FeatureLogs,
	},
}

func init() { game.Register(Definition) }

// CanonicalUID normalizes a player id. Enshrouded identifies players by
// SteamID64 — a 17-digit decimal with no case to fold — so this only
// trims. It exists (rather than being nil on the Definition) to keep the
// same seam dragonwilds needed, and as the single place to extend if a
// second id spelling ever shows up (crossplay lands with 1.0).
func CanonicalUID(uid string) string {
	return strings.TrimSpace(uid)
}
