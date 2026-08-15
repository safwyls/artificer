package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/safwyls/flamekeeper/internal/agentctl"
	"github.com/safwyls/flamekeeper/internal/flameagent"
	"github.com/safwyls/flamekeeper/internal/store"
)

// Provisioning ("new server from the dashboard"): flamekeeper deliberately
// holds no docker create rights, so this endpoint does everything short of
// placing the container — it registers a fully wired server row (host,
// game port, agent URL + token), generates a supervisor-mode stack file
// for manual deploys, and, when Ilmari is configured, asks it to place the
// container now. The agent installs the game via SteamCMD on first boot,
// seeds enshrouded_server.json, and starts the server under Wine.

type provisionRequest struct {
	Name string `json:"name"`
	// Host is where the stack will run — an address flamekeeper can reach the
	// published ports on.
	Host string `json:"host"`
	// DataPath is the host directory mounted as the install volume.
	DataPath string `json:"dataPath"`
	// GamePort is the published UDP port — Enshrouded's single queryPort,
	// which carries game traffic and the Steam query both. In-container it
	// stays at the game's own default, and the agent's API at 8811.
	GamePort  int `json:"gamePort"`
	AgentPort int `json:"agentPort"`
	// ImageTag selects the flameagent channel; default latest.
	ImageTag string `json:"imageTag"`
	// AdminPassword is generated when blank: it becomes the Keepers role
	// password in enshrouded_server.json — what an admin types at the join
	// screen to hold kick/ban rights.
	AdminPassword string `json:"adminPassword"`
	// JoinPassword becomes the default role's password. Blank means an
	// open server: anyone who finds it in the browser can join.
	JoinPassword string `json:"joinPassword"`
	// ServerName is the in-game server-browser name, enforced on every
	// start; defaults to the dashboard display name.
	ServerName string `json:"serverName"`
	// RunAs is the container user:group; defaults to the TrueNAS apps
	// user 568:568. Empty string is normalized to the default; "root"
	// omits the user line entirely.
	RunAs string `json:"runAs"`
}

var runAsPattern = regexp.MustCompile(`^\d{1,7}:\d{1,7}$`)

// controlChars matches anything that would break out of the line it is
// written on. The generated stack interpolates operator-supplied strings,
// and the result is pasted into `docker compose` on a host — so a name
// carrying a newline could append services of its own. Values that reach
// YAML as *values* are %q-quoted, which handles this; the display name
// also reaches the header comment, where quoting isn't available. Rejecting
// control characters outright is simpler than escaping per destination, and
// costs nothing real: they are meaningless in every field that has one.
var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// imageTagPattern is docker's tag grammar; anything looser could inject
// lines into the generated stack yaml.
var imageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleProvisionServer(w http.ResponseWriter, r *http.Request) {
	var req provisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.DataPath = strings.TrimSpace(req.DataPath)
	switch {
	case req.Name == "":
		writeError(w, http.StatusBadRequest, "name is required")
		return
	case req.Host == "":
		writeError(w, http.StatusBadRequest, "host is required — the address Flamekeeper will reach the server on")
		return
	case controlChars.MatchString(req.Name + req.Host + req.ServerName + req.DataPath):
		writeError(w, http.StatusBadRequest, "names and paths cannot contain line breaks or control characters")
		return
	// With a provisioner configured the data path is its call (<data
	// root>/<slug>) and the wizard doesn't even ask; a paste-flow deploy
	// has no provisioner to decide, so the operator must say.
	case req.DataPath == "" && s.Provisioner == nil:
		writeError(w, http.StatusBadRequest, "data path must be an absolute host path for the install volume")
		return
	case req.DataPath != "" && !filepath.IsAbs(req.DataPath):
		writeError(w, http.StatusBadRequest, "data path must be an absolute host path for the install volume")
		return
	}
	slug := slugify(req.Name)
	if req.DataPath == "" {
		health, err := s.Provisioner.Health(r.Context())
		if err != nil || health.Provision == nil || health.Provision.DataRoot == "" {
			writeError(w, http.StatusBadGateway,
				"the provisioner is unreachable — enter a data path to generate a stack for manual deploy instead")
			return
		}
		req.DataPath = filepath.Join(health.Provision.DataRoot, slug)
	}
	if req.GamePort == 0 {
		req.GamePort = flameagent.DefaultGamePort
	}
	if req.AgentPort == 0 {
		req.AgentPort = 8811
	}
	if req.GamePort < 1 || req.GamePort > 65535 {
		writeError(w, http.StatusBadRequest, "game port must be in 1-65535")
		return
	}
	if req.AgentPort < 1 || req.AgentPort > 65535 {
		writeError(w, http.StatusBadRequest, "agent port must be in 1-65535")
		return
	}
	if req.AgentPort == req.GamePort {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("agent port %d collides with the game port", req.AgentPort))
		return
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	if !imageTagPattern.MatchString(req.ImageTag) {
		writeError(w, http.StatusBadRequest, "image tag must match docker tag grammar")
		return
	}
	if req.AdminPassword == "" {
		req.AdminPassword = randomHex(10)
	}
	if req.ServerName == "" {
		req.ServerName = req.Name
	}
	switch {
	case req.RunAs == "":
		req.RunAs = "568:568"
	case req.RunAs == "root":
		req.RunAs = ""
	case !runAsPattern.MatchString(req.RunAs):
		writeError(w, http.StatusBadRequest, `run-as must be numeric uid:gid (or "root")`)
		return
	}
	token := randomHex(24)

	// The container is named from the slug, so a name already on the host
	// can never deploy. Catch it before anything is written: the row would
	// carry freshly generated credentials that the running container has
	// never seen, leaving a server flamekeeper can see and never reach. (The
	// provisioner refuses this itself — checked here too so an older
	// provisioner image, which only fails at docker create, is covered.)
	if s.Provisioner != nil {
		if found, err := s.Provisioner.Discover(r.Context()); err == nil {
			for _, f := range found {
				if f.Name == "flameagent-"+slug {
					writeError(w, http.StatusConflict, conflictMessage(slug))
					return
				}
			}
		}
	}

	userLine := ""
	if req.RunAs != "" {
		userLine = fmt.Sprintf(`    # The data path must be owned (or writable) by this user.
    user: "%s"
`, req.RunAs)
	}
	identityEnv := fmt.Sprintf("      FLAMEAGENT_SERVER_NAME: %q\n", req.ServerName)
	if req.JoinPassword != "" {
		identityEnv += fmt.Sprintf("      FLAMEAGENT_JOIN_PASSWORD: %q\n", req.JoinPassword)
	}

	stack := fmt.Sprintf(`# %s — Enshrouded server supervised by flameagent, generated by
# Flamekeeper. Deploy as its own stack (TrueNAS custom app / docker
# compose). On first boot the agent installs the game via SteamCMD —
# watch progress from the server's dashboard card — seeds
# enshrouded_server.json, and starts the server under Wine.
services:
  flameagent:
    image: ghcr.io/safwyls/flameagent:%s
%s    environment:
      # SteamCMD needs a writable home; the run-as user has none in the image.
      HOME: /tmp
      FLAMEAGENT_MODE: supervisor
      FLAMEAGENT_TOKEN: %s
      FLAMEAGENT_ADMIN_PASSWORD: %s
%s    ports:
      - "%d:%d/udp"   # game + Steam query (Enshrouded's single UDP port)
      - "%d:8811"     # flameagent API — the dashboard's only channel
    volumes:
      # Must be writable by the container user — uid 1000 unless user:
      # overrides it.
      - %s:/enshrouded
    restart: unless-stopped
`, req.Name, req.ImageTag, userLine, token, req.AdminPassword, identityEnv,
		req.GamePort, flameagent.DefaultGamePort,
		req.AgentPort, req.DataPath)

	// One-click (phase 5): when a provisioner is configured, deploy the
	// stack now — before registering, so a deploy that never made anything
	// leaves no server row behind. A *refusal* is fatal: the container name
	// is taken, or the token was rejected, and neither is fixed by pasting
	// the same stack somewhere. Everything else is not — a provisioner that
	// couldn't be reached, or one that created the container and failed to
	// start it, both leave a row and a stack that still describe the server
	// the operator wanted, which is the point of still generating one.
	deployed := false
	deployError := ""
	dataDir := ""
	container := ""
	if s.Provisioner != nil {
		result, err := s.Provisioner.Provision(r.Context(), flameagent.ProvisionRequest{
			Slug:          slug,
			ImageTag:      req.ImageTag,
			Token:         token,
			AdminPassword: req.AdminPassword,
			JoinPassword:  req.JoinPassword,
			ServerName:    req.ServerName,
			RunAs:         req.RunAs,
			GamePort:      req.GamePort,
			AgentPort:     req.AgentPort,
		})
		switch {
		case err == nil:
			deployed = true
			dataDir = result.DataDir
			container = result.Container
		// The only conflict /v1/provision reports is the container name.
		case errors.Is(err, agentctl.ErrBusy):
			s.logger.Warn("provisioner refused deploy: name in use", "server", req.Name, "slug", slug)
			writeError(w, http.StatusConflict, conflictMessage(slug))
			return
		case errors.Is(err, agentctl.ErrRejected):
			s.logger.Warn("provisioner refused deploy", "server", req.Name, "error", err)
			writeAgentError(w, err)
			return
		default:
			deployError = err.Error()
			s.logger.Error("provisioner deploy failed", "server", req.Name, "error", err)
		}
	}

	srv := &store.Server{
		// No RCON or REST ports: the game has neither, and everything the
		// dashboard reads arrives through the agent.
		Name: req.Name, Host: req.Host,
		GamePort:   req.GamePort,
		Enabled:    true,
		AgentURL:   fmt.Sprintf("http://%s:%d", req.Host, req.AgentPort),
		AgentToken: token,
		// Recorded only when the provisioner actually made it: this is the
		// name the destroy path passes back, and — when flamekeeper's own docker
		// proxy happens to watch the same daemon — what the container logs
		// viewer and watchdog key off. Power control is unaffected either
		// way, since every power site tries agentSupervisor before docker.
		ContainerName: container,
	}
	id, err := s.store.CreateServer(r.Context(), srv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create server")
		return
	}
	srv.ID = id
	s.audit(r, id, "server-provision", srv.Name)
	if deployed {
		s.audit(r, id, "server-deploy", container)
		s.logger.Info("provisioner deployed server", "server", srv.Name, "container", container)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"server":        toDTO(srv),
		"adminPassword": req.AdminPassword,
		"agentToken":    token,
		"stack":         stack,
		"deployed":      deployed,
		"deployError":   deployError,
		"dataDir":       dataDir,
	})
}

// conflictMessage names both ways out of a taken container name, because
// which one is right depends on what the operator meant: a genuinely new
// server needs a different name, while "I deleted the row and want it
// back" is what adoption is for.
func conflictMessage(slug string) string {
	return fmt.Sprintf("a container named flameagent-%s already exists on the host — "+
		"pick a different name, or adopt the existing container from Add server", slug)
}

// handleProvisionDefaults reports everything the wizard can prefill: the
// provisioner's own configuration (data root, public host, run-as, image
// tag) plus a free-port proposal computed from the servers flamekeeper already
// manages. The proposal is a suggestion — something else on the box can
// still hold a port, in which case the deploy fails cleanly at create
// time.
func (s *Server) handleProvisionDefaults(w http.ResponseWriter, r *http.Request) {
	if s.Provisioner == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	health, err := s.Provisioner.Health(r.Context())
	if err != nil || health.Provision == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	servers, err := s.store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}

	// Containers hold ports too — including ones whose flamekeeper row was
	// deleted. The provisioner sees them; a proposal that ignored them
	// would suggest ports that fail at deploy time.
	var containerPorts []int
	if found, err := s.Provisioner.Discover(r.Context()); err == nil {
		for _, f := range found {
			containerPorts = append(containerPorts, f.GamePort, f.AgentPort)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"host":      s.inferHost(health.Provision.PublicHost, servers),
		"dataRoot":  health.Provision.DataRoot,
		"runAs":     health.Provision.RunAs,
		"imageTag":  health.Provision.ImageTag,
		"ports":     proposePorts(servers, containerPorts),
	})
}

// inferHost picks the address for new servers: the provisioner's declared
// public host wins; else the host part of the provisioner URL when it's a
// real address (a bare compose service name — no dots, not an IP — can't
// be reached by players or by flamekeeper's REST client); else the address the
// existing servers already use.
func (s *Server) inferHost(declared string, servers []*store.Server) string {
	if declared != "" {
		return declared
	}
	if u, err := url.Parse(s.Provisioner.BaseURL()); err == nil {
		if h := u.Hostname(); strings.Contains(h, ".") {
			return h
		}
	}
	counts := map[string]int{}
	best := ""
	for _, srv := range servers {
		if srv.Host == "" {
			continue
		}
		counts[srv.Host]++
		if best == "" || counts[srv.Host] > counts[best] {
			best = srv.Host
		}
	}
	return best
}

// proposePorts finds the first offset where the game's single UDP port and
// the agent's port are both free of anything flamekeeper tracks or the
// host's containers hold.
func proposePorts(servers []*store.Server, containerPorts []int) map[string]int {
	used := map[int]bool{}
	for _, srv := range servers {
		used[srv.GamePort] = true
		if u, err := url.Parse(srv.AgentURL); err == nil {
			if p, err := strconv.Atoi(u.Port()); err == nil {
				used[p] = true
			}
		}
	}
	for _, p := range containerPorts {
		if p != 0 {
			used[p] = true
		}
	}
	for offset := 0; offset < 1000; offset++ {
		game, agent := flameagent.DefaultGamePort+offset, 8811+offset
		if !used[game] && !used[agent] {
			return map[string]int{"game": game, "agent": agent}
		}
	}
	return map[string]int{"game": flameagent.DefaultGamePort, "agent": 8811}
}

// handleProvisionDiscover surfaces flameagent containers already on the
// provisioner's host, marking the ones flamekeeper knows about so the add
// dialog offers only genuine adoptees prominently.
func (s *Server) handleProvisionDiscover(w http.ResponseWriter, r *http.Request) {
	if s.Provisioner == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "servers": []any{}})
		return
	}
	found, err := s.Provisioner.Discover(r.Context())
	if err != nil {
		writeAgentError(w, err)
		return
	}
	servers, err := s.store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}
	registeredAgentPorts := map[int]bool{}
	for _, srv := range servers {
		if u, err := url.Parse(srv.AgentURL); err == nil {
			if p, err := strconv.Atoi(u.Port()); err == nil {
				registeredAgentPorts[p] = true
			}
		}
	}
	type candidate struct {
		agentctl.DiscoveredServer
		Registered bool `json:"registered"`
	}
	out := make([]candidate, 0, len(found))
	for _, f := range found {
		out = append(out, candidate{f, f.AgentPort != 0 && registeredAgentPorts[f.AgentPort]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "servers": out})
}

// handleAdoptServer re-registers a discovered flameagent container as a
// server row — the recovery path for "the row was deleted but the
// container lives on". The provisioner returns the container's own
// registration data (secrets included, since it injected them), so
// nothing has to be dug out of the host by hand.
func (s *Server) handleAdoptServer(w http.ResponseWriter, r *http.Request) {
	if s.Provisioner == nil {
		writeError(w, http.StatusBadRequest, "no provisioner configured")
		return
	}
	var req struct {
		Container string `json:"container"`
		// Host optionally overrides the inferred address.
		Host string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Container) == "" {
		writeError(w, http.StatusBadRequest, "container name is required")
		return
	}

	adopted, err := s.Provisioner.Adopt(r.Context(), strings.TrimSpace(req.Container))
	if err != nil {
		writeAgentError(w, err)
		return
	}
	if adopted.Token == "" || adopted.AgentPort == 0 {
		writeError(w, http.StatusBadRequest, "that container has no agent token or published agent port — add it manually")
		return
	}

	servers, err := s.store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		health, err := s.Provisioner.Health(r.Context())
		declared := ""
		if err == nil && health.Provision != nil {
			declared = health.Provision.PublicHost
		}
		host = s.inferHost(declared, servers)
	}
	if host == "" {
		writeError(w, http.StatusBadRequest,
			"could not infer the host address — set FLAMEAGENT_PUBLIC_HOST on the provisioner or pass one")
		return
	}

	name := adopted.ServerName
	if name == "" {
		name = strings.ReplaceAll(strings.TrimPrefix(adopted.Name, "flameagent-"), "-", " ")
	}
	srv := &store.Server{
		Name: name, Host: host,
		GamePort:      adopted.GamePort,
		Enabled:       true,
		AgentURL:      fmt.Sprintf("http://%s:%d", host, adopted.AgentPort),
		AgentToken:    adopted.Token,
		ContainerName: adopted.Name,
	}
	id, err := s.store.CreateServer(r.Context(), srv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create server")
		return
	}
	srv.ID = id
	s.audit(r, id, "server-adopt", adopted.Name)
	s.logger.Info("adopted server", "container", adopted.Name, "server", name)
	writeJSON(w, http.StatusCreated, map[string]any{"server": toDTO(srv)})
}

// slugify reduces a display name to a container/directory-safe slug.
func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "enshrouded"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return strings.Trim(slug, "-")
}
