// Package dragonwilds implements wildskeeper's game contract for RuneScape:
// Dragonwilds dedicated servers.
//
// The game has no RCON, no HTTP admin API and no query protocol — all
// native administration is the in-game Server Management menu. Everything
// wildskeeper shows is therefore *derived*: process liveness from the wkagent
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
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/safwyls/artificer/core/game"
	"github.com/safwyls/artificer/games/dragonwilds/dwconfig"
)

// AppID is the Steam app id of the *dedicated server* tool (the game client
// is 1374490). Deliberately duplicated in wkagent's Dragonwilds launch
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
	// The settings editor speaks DedicatedServer.ini through dwconfig.
	Config: &game.ConfigCodec{
		Filename:      "DedicatedServer.ini",
		NotConfigured: dwconfig.ErrNotConfigured,
		Read: func(path string) (*game.ConfigPayload, error) {
			res, err := dwconfig.Read(path)
			if err != nil {
				return nil, err
			}
			return &game.ConfigPayload{Settings: res.Settings, Path: res.Path, Writable: res.Writable}, nil
		},
		Write:               dwconfig.Write,
		RotateAdminPassword: dwconfig.RotateAdminPassword,
	},
	// The world is the newest .sav; everything in the save dir is
	// archived and the size floor is verification enough (the format is
	// GVAS but a magic check adds nothing the floor doesn't).
	Save: &game.SaveLayout{
		WorldFile: newestSav,
	},
}

func init() { game.Register(Definition) }

// playerIDPattern is the shape a real Player ID takes: 32 hex characters,
// the EOS ProductUserId form. Confirmed against a live account, and
// matched case-insensitively on purpose — see CanonicalUID.
var playerIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// CanonicalUID lowercases a 32-hex player id and trims everything else.
//
// The case-folding is not a guess. The game renders the same 32-hex shape
// in two different cases depending on where it appears: the in-game
// Settings screen shows a Player ID lowercase, while the values the server
// writes itself are uppercase (`ServerGuid=6E8B93DD...` in the ini,
// `WorldSaveGuid` uppercase in the log). An id that arrives from one place
// and is matched against the other would silently never match — which for
// a visibility check means failing open, the exact failure the porting doc
// warns about.
//
// Anything that isn't 32 hex characters is only trimmed. Lowercasing hex
// is lossless and cannot collide two distinct ids; lowercasing an unknown
// format could, so unrecognised values are left alone rather than mangled
// into something that matches nothing.
func CanonicalUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if playerIDPattern.MatchString(uid) {
		return strings.ToLower(uid)
	}
	return uid
}

// newestSav finds the most recently written .sav — Dragonwilds worlds
// are ordinary UE .sav files, so the extension is trustworthy here.
func newestSav(saveDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(saveDir, "*.sav"))
	if err != nil || len(matches) == 0 {
		return "", errors.New("the save directory holds no .sav files — has the server saved yet?")
	}
	best, bestMod := "", int64(-1)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); mod > bestMod {
			best, bestMod = m, mod
		}
	}
	return best, nil
}
