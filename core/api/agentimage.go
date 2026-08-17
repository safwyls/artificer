package api

import (
	"encoding/json"
	"net/http"
)

// handleRecreateAgent moves a provisioned server's agent onto a different
// flameagent image — a channel switch (latest to a pinned tag, say)
// without touching the world.
//
// This exists because provisioned containers belong to no orchestrator:
// they don't appear in a TrueNAS apps list or any compose file, so
// changing their image otherwise means hand-writing docker commands on
// the host. Anvil placed them and can rebuild them, which makes this a
// button instead of a runbook.
// handleGetLaunch reports how the agent will start the game.
//
// Read-only, and nothing selects it: Enshrouded ships one build, so the
// launch *chooser* was removed. What survives is worth reading — whether
// the game's files are present, and whether this agent's image can run
// them at all, which is the one failure a rebuild fixes.
func (s *Server) handleGetLaunch(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	agent := s.agentFor(srv)
	if agent == nil {
		writeError(w, http.StatusBadRequest, "no agent configured for this server")
		return
	}
	health, err := agent.Health(r.Context())
	if err != nil {
		writeAgentError(w, err)
		return
	}
	if health.Launch == nil {
		// Companion mode: nothing is launched here, so there is nothing to
		// report. A 400 rather than an empty object, matching how the other
		// supervisor-only verbs answer.
		writeError(w, http.StatusBadRequest, "this agent does not run the game")
		return
	}
	writeJSON(w, http.StatusOK, health.Launch)
}

func (s *Server) handleRecreateAgent(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		ImageTag string `json:"imageTag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if s.Provisioner == nil {
		writeError(w, http.StatusBadRequest,
			"no provisioner is configured, so Flametender cannot rebuild this container — change its image where it was deployed")
		return
	}
	if srv.ContainerName == "" {
		writeError(w, http.StatusBadRequest, "this server has no container name recorded, so there is nothing to rebuild")
		return
	}
	result, err := s.Provisioner.RecreateAgent(r.Context(), srv.ContainerName, req.ImageTag)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	s.audit(r, srv.ID, "agent-image", result.Image)
	writeJSON(w, http.StatusOK, result)
}
