package palagent_test

import (
	"testing"

	"github.com/safwyls/dwcon/internal/games/dragonwilds"
	"github.com/safwyls/dwcon/internal/palagent"
)

// The agent spells the Steam app id out itself rather than importing the
// game package, so that a thin sidecar doesn't link the game registry it
// never uses (see palagent.DefaultAppID). This test is where the two are
// held together — a test-only import, so it costs the shipped binary
// nothing.
func TestDefaultAppIDMatchesDragonwilds(t *testing.T) {
	if palagent.DefaultAppID != dragonwilds.AppID {
		t.Errorf("palagent.DefaultAppID = %d, dragonwilds.AppID = %d — the agent would update the wrong app",
			palagent.DefaultAppID, dragonwilds.AppID)
	}
}
