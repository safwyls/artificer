package palworld_test

import (
	"testing"

	"github.com/safwyls/sampo/games/palworld"
	"github.com/safwyls/sampo/games/palworld/palagent"
)

// The app id and default port are deliberately spelled twice — console-
// side game module and agent-side spec — so the agent binary never links
// the game registry. This agreement test keeps the copies honest.
func TestAgentAndConsoleAgreeOnTheAppID(t *testing.T) {
	if palworld.AppID != palagent.AppID {
		t.Fatalf("console AppID %d != agent AppID %d", palworld.AppID, palagent.AppID)
	}
	if palworld.Definition.DefaultGamePort != palagent.DefaultGamePort {
		t.Fatalf("console port %d != agent port %d", palworld.Definition.DefaultGamePort, palagent.DefaultGamePort)
	}
	g := palagent.Game()
	if g.AppID != palagent.AppID || g.DefaultGamePort != palagent.DefaultGamePort {
		t.Fatalf("spec disagrees with its own constants: %+v", g)
	}
}
