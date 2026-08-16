package agent

import (
	"log/slog"
	"net/http"
)

// The game-facing surface for Game.Routes handlers — like core/api's
// game surface, deliberately small.

// WriteJSON writes a JSON response the way the kit's own handlers do.
func WriteJSON(w http.ResponseWriter, status int, v any) { writeJSON(w, status, v) }

// WriteError writes the kit's error shape.
func WriteError(w http.ResponseWriter, status int, msg string) { writeError(w, status, msg) }

// SupervisedStatus returns the supervised game's status; ok=false in
// companion mode, where the agent launches nothing.
func (a *Agent) SupervisedStatus() (GameStatus, bool) {
	if a.game == nil {
		return GameStatus{}, false
	}
	return a.game.Status(), true
}

// SupervisedGamePort is the port the supervisor enforces/publishes for
// the game; zero in companion mode.
func (a *Agent) SupervisedGamePort() int {
	if a.game == nil {
		return 0
	}
	return a.game.gamePort
}

// SupervisedProfile is the active launch profile; ok=false in companion
// mode.
func (a *Agent) SupervisedProfile() (Profile, bool) {
	if a.game == nil {
		return Profile{}, false
	}
	return a.game.Profile(), true
}

// SupervisedRunning reports whether the supervised game is up; false in
// companion mode.
func (a *Agent) SupervisedRunning() bool {
	return a.game != nil && a.game.Running()
}

// InstallDir is the game install root the agent holds.
func (a *Agent) InstallDir() string { return a.cfg.InstallDir }

// LoggerHandle is the agent's logger, for game routes.
func (a *Agent) LoggerHandle() *slog.Logger { return a.cfg.Logger }
