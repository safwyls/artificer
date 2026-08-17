package dragonwilds_test

import (
	"testing"

	"github.com/safwyls/artificer/games/dragonwilds"
	"github.com/safwyls/artificer/games/dragonwilds/dwagent"
)

// The app id and default port are deliberately spelled twice — console-
// side game module and agent-side spec — so the agent binary never links
// the game registry. This agreement test keeps the copies honest.
func TestAgentAndConsoleAgreeOnTheAppID(t *testing.T) {
	if dragonwilds.AppID != dwagent.AppID {
		t.Fatalf("console AppID %d != agent AppID %d", dragonwilds.AppID, dwagent.AppID)
	}
	if dragonwilds.Definition.DefaultGamePort != dwagent.DefaultGamePort {
		t.Fatalf("console port %d != agent port %d", dragonwilds.Definition.DefaultGamePort, dwagent.DefaultGamePort)
	}
	g := dwagent.Game(dwagent.LaunchConfig{})
	if g.AppID != dwagent.AppID || g.DefaultGamePort != dwagent.DefaultGamePort {
		t.Fatalf("spec disagrees with its own constants: %+v", g)
	}
}
