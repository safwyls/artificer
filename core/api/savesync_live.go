package api

// Two supporting surfaces for the vault's page: custody events as they
// happen, and game artwork.
//
// Events are server-sent, not polled. A friend group watching a world
// change hands should see it change hands — a 20-second poll makes the
// hand-off feel broken even when it worked instantly. SSE is the right
// size for this: one direction, plain HTTP, no protocol upgrade to get
// through a tunnel.
//
// Artwork is a proxy in front of IGDB so the credentials live on one
// deployment rather than on every player's machine, and so a shelf of
// covers costs IGDB one lookup for the whole group rather than one per
// companion.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/safwyls/artificer/core/igdb"
)

// syncEventKeepalive paces the comment frames that keep an idle stream
// from being reaped by a proxy. Comfortably under the 60s most default
// idle timeouts use.
const syncEventKeepalive = 25 * time.Second

// handleSyncEvents streams custody changes as server-sent events. The
// payload is deliberately thin — which world, what kind — because the
// client re-reads the truth from the API; that keeps a dropped or
// out-of-order event harmless.
func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "this server cannot stream events")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tell an nginx-shaped proxy not to buffer us; without it the stream
	// arrives in silent chunks and looks dead.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	events, unsubscribe := s.SaveSync.Subscribe()
	defer unsubscribe()

	// An opening frame proves the stream is live before anything happens,
	// so the page can show "live" rather than waiting for the first
	// custody change to find out.
	fmt.Fprintf(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(syncEventKeepalive)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case ev, ok := <-events:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: custody\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

// handleSyncArtwork resolves cover art for a batch of games. Absent
// entries mean IGDB knows nothing (or isn't configured) — never an
// error, because artwork is decoration and a shelf without covers is
// still a shelf.
func (s *Server) handleSyncArtwork(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Games []igdb.Query `json:"games"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// A generous cap: a big Steam library is a few hundred games, and the
	// lookup itself bounds how many reach IGDB.
	if len(in.Games) > 500 {
		in.Games = in.Games[:500]
	}
	art := s.Artwork.Lookup(r.Context(), in.Games)
	if art == nil {
		art = map[string]igdb.Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"available": s.Artwork.Configured(),
		"art":       art,
	})
}
