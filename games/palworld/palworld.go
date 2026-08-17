// Package palworld implements the game contracts for a Palworld dedicated
// server, over its REST API (preferred, JSON over HTTP) or Source RCON
// (fallback, for servers with the REST API disabled or on older builds).
//
// Everything Palworld-specific lives under this directory: the command
// vocabulary here, the save reader in save/, and the settings-file editor in
// config/. Nothing above internal/game knows any of it exists.
package palworld

import (
	"strconv"
	"strings"

	"github.com/safwyls/artificer/core/game"
	"github.com/safwyls/artificer/games/palworld/palconfig"
)

// AppID is the Steam app id of the Palworld dedicated server. The agent
// binary keeps its own copy (palagent.DefaultAppID) rather than linking the
// dashboard's client machinery for one number — an agreement test keeps the
// two from drifting.
const AppID = 2394010

// Definition registers Palworld with the shared layer.
var Definition = &game.Definition{
	ID:              "palworld",
	Name:            "Palworld",
	DefaultGamePort: 8211,
	NewClient:       New,
	CanonicalUID:    CanonicalUID,
	Features: []string{
		game.FeatureMap, game.FeaturePals, game.FeatureInventory,
		game.FeatureStorage, game.FeaturePaldex, game.FeatureAchievements,
		game.FeatureGuilds, game.FeatureCalculators,
	},
	// The settings editor speaks PalWorldSettings.ini through palconfig.
	Config: &game.ConfigCodec{
		Filename:      "PalWorldSettings.ini",
		NotConfigured: palconfig.ErrNotConfigured,
		Read: func(path string) (*game.ConfigPayload, error) {
			res, err := palconfig.Read(path)
			if err != nil {
				return nil, err
			}
			return &game.ConfigPayload{Settings: res.Settings, Path: res.Path, Writable: res.Writable}, nil
		},
		Write: palconfig.Write,
		// Palworld's admin access is RCON/REST credentials, not an
		// ini-rotated password session.
	},
	// Level.sav is the world; only .sav files belong in an archive; and
	// the magic check is the one mid-write guard a non-atomic writer
	// gets (drift ledger seam 1 — the guard a naive take-F would drop).
	Save: &game.SaveLayout{
		WorldFile:        levelSavPath,
		IncludeInArchive: func(rel string) bool { return strings.HasSuffix(strings.ToLower(rel), ".sav") },
		VerifyWorld:      verifySavMagic,
	},
}

func init() { game.Register(Definition) }

// New builds a client for the given server, using REST when preferred and
// falling back to RCON if the REST API is unreachable.
func New(conn game.Conn) game.Client {
	rcon := &RCONClient{
		addr:     addr(conn.Host, conn.RCONPort),
		password: conn.RCONPassword,
	}
	if !conn.PreferREST {
		return rcon
	}

	rest := &RESTClient{
		baseURL:  "http://" + addr(conn.Host, conn.RESTPort),
		password: conn.RESTPassword,
	}
	return &fallbackClient{primary: rest, fallback: rcon}
}

func addr(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}
