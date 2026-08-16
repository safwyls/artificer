package enshrouded_test

import (
	"testing"

	"github.com/safwyls/sampo/games/enshrouded"
	"github.com/safwyls/sampo/games/enshrouded/esagent"
)

// The app id is deliberately spelled twice — the console-side game
// module and the agent-side spec — so the agent binary never links the
// game registry. This is the agreement test that keeps the copies honest.
func TestAgentAndConsoleAgreeOnTheAppID(t *testing.T) {
	if enshrouded.AppID != esagent.AppID {
		t.Fatalf("console AppID %d != agent AppID %d", enshrouded.AppID, esagent.AppID)
	}
	if enshrouded.DefaultQueryPort != esagent.DefaultQueryPort {
		t.Fatalf("console port %d != agent port %d", enshrouded.DefaultQueryPort, esagent.DefaultQueryPort)
	}
	if g := esagent.Game(esagent.WineConfig{}); g.AppID != esagent.AppID || g.DefaultGamePort != esagent.DefaultQueryPort {
		t.Fatalf("spec disagrees with its own constants: %+v", g)
	}
}
