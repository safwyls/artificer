package agentctl_test

// An agent older than the verb being asked for. The taxonomy claimed to
// cover this through a JSON-less 404, but chi answers 405 whenever the
// path exists for another method — which is exactly the restore pair's
// shape, since /v1/files/save has served GET since the first agent. The
// result was a console that could only say "agent returned 405:".

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/agentctl"
)

func TestOldAgentAnswersUnsupported(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		// The pre-restore-pair surface: GET only.
		r.Get("/files/save", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("tar")) })
	})
	// A path this agent has never heard of at all — the 404 shape.
	srv := httptest.NewServer(r)
	defer srv.Close()

	c, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.SaveETag(context.Background()); !errors.Is(err, agentctl.ErrUnsupported) {
		t.Errorf("SaveETag: %v, want ErrUnsupported", err)
	} else if !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("SaveETag: %v must also be ErrRejected — callers that only know the old sentinel still work", err)
	}
	if err := c.RestoreSave(context.Background(), http.NoBody, `"whatever"`); !errors.Is(err, agentctl.ErrUnsupported) {
		t.Errorf("RestoreSave: %v, want ErrUnsupported", err)
	}
	// The JSON round-trip half of the client has the same hole.
	if _, err := c.ClearSteamCache(context.Background()); !errors.Is(err, agentctl.ErrUnsupported) {
		t.Errorf("ClearSteamCache against an agent without it: %v, want ErrUnsupported", err)
	}
}

// A 404 the agent's own handler wrote is a real answer about a missing
// thing, not a missing verb — folding the two together would tell an
// operator to update an image that is already current.
func TestHandlerNotFoundIsNotUnsupported(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Get("/files/config", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"DedicatedServer.ini not found under the install dir"}`))
		})
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	c, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetConfig(context.Background())
	if errors.Is(err, agentctl.ErrUnsupported) {
		t.Errorf("GetConfig: %v was read as a missing verb; the agent answered about a missing file", err)
	}
	if !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("GetConfig: %v, want ErrRejected", err)
	}
}
