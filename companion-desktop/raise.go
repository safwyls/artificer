package main

// The second-launch handoff.
//
// Double-clicking the exe while a companion is already running is a
// normal thing to do — usually because the window is hidden in the tray
// and the player has forgotten it is there. The running instance must
// raise its window, and the new process must bow out: two of them over
// one config file would fight over custody.
//
// The mechanism is one additive route, POST /api/raise, on the loopback
// address every build already serves. Additive is allowed; removing a
// route is not. Old builds — the browser one, the Wails one — answer
// 404 to it harmlessly, and the caller falls back to opening the page,
// which is exactly the right thing for a build whose UI *is* the page.

import (
	"context"
	"net/http"
	"time"
)

// routes is the frozen surface plus this build's one addition.
func (u *ui) routes() http.Handler {
	api := u.engine.Routes()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/raise", func(w http.ResponseWriter, r *http.Request) {
		u.showWindow()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"raised":true}`))
	})
	// Everything else — the page, its assets, every /api route — is the
	// engine's, unchanged.
	mux.Handle("/", api)
	return mux
}

// raiseRunningInstance asks the companion already running to show its
// window, and reports whether one did. A false answer means the running
// build has no window to raise (or is too old to know the route), and
// the caller should open the page instead.
func raiseRunningInstance() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+listenAddr+"/api/raise", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
