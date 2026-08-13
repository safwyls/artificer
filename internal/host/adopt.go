package host

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/safwyls/ilmari/internal/dockerctl"
)

// Discovery and adoption: how a console recovers servers it no longer has
// rows for — a reinstalled console, a deleted row, a stack that was pasted
// and deployed by hand.
//
// Which containers a console may discover follows from what it registered:
// containers it owns (by Ilmari or legacy label), plus unmanaged containers
// whose image falls under its own allowlist. The second half matters for
// paste-flow deploys — a hand-deployed wkagent stack carries no ownership
// label at all, and the image prefix is the only honest signal of whose
// kind of container it is. A container owned by *another* console is never
// discoverable, whatever its image.

// DiscoveredContainer is one candidate for adoption. Deliberately free of
// environment values — those are adopt's business, behind its own scoping.
type DiscoveredContainer struct {
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	Running bool      `json:"running"`
	Managed bool      `json:"managed"`
	Ports   []PortMap `json:"ports,omitempty"`
}

// discoverable reports whether one container is c's to see.
func (s *Service) discoverable(summary *dockerctl.ContainerSummary, c *client) bool {
	if owner, _, ok := ownerOf(summary.Labels); ok {
		return owner == c.ID
	}
	for _, prefix := range s.allowlistFor(c) {
		if strings.HasPrefix(summary.Image, prefix) {
			return true
		}
	}
	return false
}

func (s *Service) handleDiscover(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	containers, err := s.docker.ContainerList(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []DiscoveredContainer{}
	for i := range containers {
		if !s.discoverable(&containers[i], c) {
			continue
		}
		_, managed := func() (string, bool) { o, _, ok := ownerOf(containers[i].Labels); return o, ok }()
		row := DiscoveredContainer{
			Name:    containers[i].Name,
			Image:   containers[i].Image,
			Running: containers[i].State == "running",
			Managed: managed,
			Ports:   publishedPorts(containers[i].Ports),
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// AdoptResult carries what a console needs to re-register a server it lost
// the row for — including secrets from the container's environment.
//
// Returning env at all deserves its justification written down, because
// everywhere else in this service refuses to. The consoles' own
// provisioners injected these values when they created the containers, and
// the (token-authenticated) console is the party that supplied them — so
// handing them back stays inside the original trust boundary, exactly as
// the per-console provisioners did before Ilmari existed. What is new here
// is the scoping: only variables under the caller's registered env prefix
// cross, so a wildskeeper token recovers WKAGENT_* and nothing else, even
// from a container it owns. A client registered without an env prefix gets
// no environment back at all.
type AdoptResult struct {
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Running bool              `json:"running"`
	Ports   []PortMap         `json:"ports,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func (s *Service) handleAdopt(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	var req struct {
		Container string `json:"container"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Container == "" {
		writeError(w, http.StatusBadRequest, "container name is required")
		return
	}
	containers, err := s.docker.ContainerList(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	for i := range containers {
		if containers[i].Name != req.Container {
			continue
		}
		// Adoption is gated exactly like discovery: a console can only
		// recover what it could see. Foreign gets the same 403 as destroy
		// and rebuild; a container that is neither owned nor image-matched
		// is simply not this service's to hand over.
		if owner, _, ok := ownerOf(containers[i].Labels); ok && owner != c.ID {
			writeError(w, http.StatusForbidden, errForeign.Error())
			return
		}
		if !s.discoverable(&containers[i], c) {
			writeError(w, http.StatusBadRequest, "that container is not one this console could have deployed")
			return
		}
		res := AdoptResult{
			Name:    containers[i].Name,
			Image:   containers[i].Image,
			Running: containers[i].State == "running",
			Ports:   publishedPorts(containers[i].Ports),
		}
		if c.EnvPrefix != "" {
			env, err := s.docker.InspectEnv(r.Context(), containers[i].ID)
			if err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
			res.Env = map[string]string{}
			for _, e := range env {
				key, value, ok := strings.Cut(e, "=")
				if ok && strings.HasPrefix(key, c.EnvPrefix) {
					res.Env[key] = value
				}
			}
		}
		s.cfg.Logger.Info("adopted container", "container", req.Container, "client", c.ID)
		writeJSON(w, http.StatusOK, res)
		return
	}
	writeError(w, http.StatusNotFound, "no container with that name")
}

// publishedPorts flattens docker's port map into the spec's shape.
func publishedPorts(ports map[string]int) []PortMap {
	var out []PortMap
	for containerSide, hostPort := range ports {
		if hostPort == 0 {
			continue
		}
		port, proto := splitPortSpec(containerSide)
		out = append(out, PortMap{Host: hostPort, Container: port, Proto: proto})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}
