package flameagent_test

import (
	"testing"

	"github.com/safwyls/flamekeeper/internal/flameagent"
	"github.com/safwyls/flamekeeper/internal/games/enshrouded"
)

// The agent spells the Steam app id and the game's UDP port out itself
// rather than importing the game package, so that a thin sidecar doesn't
// link the game registry it never uses (see flameagent.DefaultAppID).
// This test is where the two are held together — a test-only import, so
// it costs the shipped binary nothing.
func TestDefaultAppIDMatchesEnshrouded(t *testing.T) {
	if flameagent.DefaultAppID != enshrouded.AppID {
		t.Errorf("flameagent.DefaultAppID = %d, enshrouded.AppID = %d — the agent would update the wrong app",
			flameagent.DefaultAppID, enshrouded.AppID)
	}
	if flameagent.DefaultGamePort != enshrouded.DefaultQueryPort {
		t.Errorf("flameagent.DefaultGamePort = %d, enshrouded.DefaultQueryPort = %d — provisioning would publish the wrong port",
			flameagent.DefaultGamePort, enshrouded.DefaultQueryPort)
	}
}
