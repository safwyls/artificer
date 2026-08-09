// Package dragonwilds implements palcon's game contract for RuneScape:
// Dragonwilds dedicated servers.
//
// The game has no RCON, no HTTP admin API and no query protocol — all
// native administration is the in-game Server Management menu. Everything
// palcon shows is therefore *derived*: process liveness from the palagent
// sidecar's health, the player list from a state machine over the agent's
// log tail (dwlog), config from DedicatedServer.ini (dwconfig). Commands
// have no transport at all until the UE4SS command bridge exists, so the
// command methods return game.UnsupportedError and the UI says why instead
// of offering buttons that 502.
//
// docs/dragonwilds-recon.md is the source for every external fact used
// here, and marks which of them are still unverified.
package dragonwilds

import (
	"strings"

	"github.com/safwyls/dwcon/internal/game"
)

// AppID is the Steam app id of the *dedicated server* tool (the game client
// is 1374490). Deliberately duplicated in palagent's Dragonwilds launch
// profile the same way palworld.AppID is — agreement tests keep them honest.
const AppID = 4019830

// MaxPlayers is the game's hard cap — there is no config key for it. The
// frontend's six-segment sigil and the memory sizing hint both key off it.
const MaxPlayers = 6

// Definition registers Dragonwilds with the shared layer. The feature list
// is deliberately small and honest: players (the Adventurers view), world
// saves, and the live log tail. No map, no inventories — the save format is
// unparsed (recon: Phase 3 gate) — and no bans view until a ban list is
// reachable at rest or over a bridge.
var Definition = &game.Definition{
	ID:              "dragonwilds",
	Name:            "RuneScape: Dragonwilds",
	DefaultGamePort: 7777,
	NewClient:       New,
	CanonicalUID:    CanonicalUID,
	Features: []string{
		game.FeaturePals,
		game.FeatureSaves,
		game.FeatureLogs,
	},
}

func init() { game.Register(Definition) }

// CanonicalUID trims a player id and nothing more. The id's wire format is
// undocumented (recon: "Player identity", UNVERIFIED) and v0 log lines
// carry names rather than ids, so there are no divergent spellings to
// reconcile yet. Guessing a normalization (case-folding, dash-stripping)
// against an unknown format is how a visibility check fails open — the
// porting doc's warning — so until real ids are captured, identity is the
// only correct transform.
func CanonicalUID(uid string) string { return strings.TrimSpace(uid) }
