package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/safwyls/wildskeeper/internal/game"
	"github.com/safwyls/wildskeeper/internal/store"

	// The registry is populated by importing this, exactly as the binary
	// does. Without it every server row resolves to "unknown game" — a
	// failure no compiler catches, which is what the tests below are for.
	_ "github.com/safwyls/wildskeeper/internal/games"
)

// A server row must resolve to a live client. This is the wiring the binary
// depends on: an empty registry compiles fine and then 501s every request.
func TestServerResolvesToAGameClient(t *testing.T) {
	app, _ := newTestAppWithAdmin(t)

	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "s1", Host: "127.0.0.1", RCONPort: 25575, RCONPassword: "pw", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := app.store.GetServer(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	// A row created without naming a game defaults to Palworld, so existing
	// installs keep working across the migration that added the column.
	if srv.Game != game.DefaultID {
		t.Errorf("game = %q, want %q", srv.Game, game.DefaultID)
	}
	def, ok := srv.Definition()
	if !ok {
		t.Fatalf("no definition registered for %q", srv.Game)
	}
	if def.DefaultGamePort == 0 || !def.HasFeature(game.FeaturePals) {
		t.Errorf("definition looks unpopulated: %+v", def)
	}
	if _, err := srv.Client(); err != nil {
		t.Fatalf("building a client: %v", err)
	}
}

// A row naming a game this build doesn't have is a downgrade or a hand-edited
// row, not a bad request — it must be reported as unimplemented rather than
// as a missing server or a 500.
func TestUnknownGameIsNotImplemented(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "future", Game: "ark", Host: "127.0.0.1", RCONPort: 25575, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := app.do(t, "GET", "/api/servers/"+itoa(id)+"/info", nil, admin)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("unknown game info: got %d, want 501", rec.Code)
	}
}

// The canonical-uid hook has to come from the server's own game: getting it
// wrong fails open in a visibility check rather than erroring.
func TestCanonicalUIDComesFromTheGame(t *testing.T) {
	// Dragonwilds normalizes by trimming only — the id's wire format is
	// undocumented, so anything more would be invented (recon doc).
	srv := &store.Server{Game: game.DefaultID}
	if got := srv.CanonicalUID("  AbC-123  "); got != "AbC-123" {
		t.Errorf("CanonicalUID = %q, want the trimmed id", got)
	}
	// An unknown game must pass the id through rather than mangle it.
	unknown := &store.Server{Game: "ark"}
	if got := unknown.CanonicalUID("abc"); got != "abc" {
		t.Errorf("unknown game CanonicalUID = %q, want the id untouched", got)
	}
}
