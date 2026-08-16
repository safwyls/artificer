package agent

import "net/http"

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
