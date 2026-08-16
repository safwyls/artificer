package flameagent

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/safwyls/flametender/internal/games/enshrouded/esquery"
)

// The Steam query, run here rather than from the console.
//
// The roadmap originally had flametender querying `host:gamePort`
// itself. Two things argue for the agent instead. The standing constraint
// is that the agent is the only transport (docs/architecture.md), and a
// second path from the console to the host would be the first exception.
// More practically, whether A2S even answers from off-host is still an
// open row in the recon ledger — a NAT'd or firewalled deployment might
// publish the port to players and not to the console. From here the query
// goes to the loopback address inside the container, which is the one
// place it is known to work (mornedhels polls 127.0.0.1:queryPort the
// same way), so the phase stops waiting on a fact it doesn't need.
//
// The port is the supervisor's own `gamePort` — the same number enforced
// into enshrouded_server.json's queryPort before each start — so this
// cannot drift from what the game bound.

// queryTimeout bounds the whole handler. The console calls this on its
// info path, so a server that has bound its port but isn't answering yet
// (the gap between process start and `HostOnline`) must fail fast rather
// than hold a dashboard request open.
const queryTimeout = 3 * time.Second

// handleQuery answers with the game's own A2S reply.
//
// A server that doesn't answer is a 503 with the reason, not a 500: it is
// the ordinary state of a game that is still booting, and the console
// renders it as "no query answer" rather than as an agent fault.
func (a *Agent) handleQuery(w http.ResponseWriter, r *http.Request) {
	if a.game == nil {
		writeError(w, http.StatusBadRequest, "agent is in companion mode — it does not know the game's query port")
		return
	}
	if st := a.game.Status(); st.State != "running" {
		writeError(w, http.StatusServiceUnavailable, "the game process is "+st.State)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(a.game.gamePort))

	info, err := esquery.QueryInfo(ctx, addr)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the game did not answer the Steam query: "+err.Error())
		return
	}
	// Players are a second round trip and a lesser prize — the count is
	// already in the info reply, and A2S player rows carry no account id.
	// A failure here costs the names, not the answer.
	players, perr := esquery.QueryPlayers(ctx, addr)
	res := map[string]any{"info": info, "players": players}
	if perr != nil {
		res["playersError"] = perr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}
