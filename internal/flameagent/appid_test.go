package flameagent_test

import (
	"testing"

	"github.com/safwyls/flamekeeper/internal/games/dragonwilds"
	"github.com/safwyls/flamekeeper/internal/flameagent"
)

// The agent spells the Steam app id out itself rather than importing the
// game package, so that a thin sidecar doesn't link the game registry it
// never uses (see flameagent.DefaultAppID). This test is where the two are
// held together — a test-only import, so it costs the shipped binary
// nothing.
func TestDefaultAppIDMatchesDragonwilds(t *testing.T) {
	if flameagent.DefaultAppID != dragonwilds.AppID {
		t.Errorf("flameagent.DefaultAppID = %d, dragonwilds.AppID = %d — the agent would update the wrong app",
			flameagent.DefaultAppID, dragonwilds.AppID)
	}
}
