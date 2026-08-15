package api

import (
	"encoding/json"
	"net/http"
)

// Launch profile: how the agent starts the game. Enshrouded ships one
// build (Windows, run under Wine), so today this is read-mostly — the
// selectable list holds a single entry, and a hand-configured custom
// command reports itself here. The endpoint stays because it is the seam
// a second build (a native Linux server at 1.0?) would arrive through,
// and because "what is this agent actually going to run" is worth being
// able to ask.

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
		// Companion or provisioner mode: nothing is being launched here, so
		// there is no build to choose. A 400 rather than an empty object,
		// matching how the other supervisor-only verbs answer.
		writeError(w, http.StatusBadRequest, "this agent does not run the game, so it has no launch profile")
		return
	}
	writeJSON(w, http.StatusOK, health.Launch)
}

func (s *Server) handleSetLaunch(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	agent := s.agentFor(srv)
	if agent == nil {
		writeError(w, http.StatusBadRequest, "no agent configured for this server")
		return
	}
	status, err := agent.SetLaunchProfile(r.Context(), req.Profile)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	// Worth auditing: it changes what the next game start runs.
	s.audit(r, srv.ID, "launch-profile", req.Profile)
	writeJSON(w, http.StatusOK, status)
}

// handleRecreateAgent moves a provisioned server's agent onto a different
// flameagent image — a channel switch (latest to a pinned tag, say)
// without touching the world.
//
// This exists because provisioned containers belong to no orchestrator:
// they don't appear in a TrueNAS apps list or any compose file, so
// changing their image otherwise means hand-writing docker commands on
// the host. Ilmari placed them and can rebuild them, which makes this a
// button instead of a runbook.
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
