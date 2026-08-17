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

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/store"
)

// Provisioning ("new server from the dashboard"): the console deliberately
// holds no docker create rights, so this endpoint does everything short of
// placing the container — it registers a fully wired server row (host,
// game port, agent URL + token), generates a supervisor-mode stack file
// for manual deploys, and, when Anvil is configured, asks it to place the
// container now. Everything game-shaped it interpolates comes from the
// console's ProvisionProfile.

type provisionRequest struct {
	Name string `json:"name"`
	// Host is where the stack will run — an address flametender can reach the
	// published ports on.
	Host string `json:"host"`
	// DataPath is the host directory mounted as the install volume.
	DataPath string `json:"dataPath"`
	// GamePort is the published game port. In-container it stays at the
	// game's own default, and the agent's API at 8811.
	GamePort int `json:"gamePort"`
	// RESTPort/RCONPort publish the named admin transports, for games
	// whose profile declares them (Palworld). Ignored otherwise.
	RESTPort int `json:"restPort"`
	RCONPort int `json:"rconPort"`
	// ServerDesc is the server-browser description, for games that have
	// one.
	ServerDesc string `json:"serverDesc"`
	AgentPort  int    `json:"agentPort"`
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
	// OwnerID is the in-game identity that owns the server; required
	// when the game's profile says so (some games refuse to start
	// without an owner).
	OwnerID string `json:"ownerId"`
	// WorldName names the world created on first boot, for games that
	// distinguish it from the server-browser name.
	WorldName string `json:"worldName"`
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
	p := s.Provision
	if p == nil {
		writeError(w, http.StatusNotImplemented, "provisioning is not wired for this console")
		return
	}
	var req provisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.DataPath = strings.TrimSpace(req.DataPath)
	req.OwnerID = strings.TrimSpace(req.OwnerID)
	switch {
	case req.Name == "":
		writeError(w, http.StatusBadRequest, "name is required")
		return
	case req.Host == "":
		writeError(w, http.StatusBadRequest, "host is required — the address the console will reach the server on")
		return
	case p.OwnerIDRequired && req.OwnerID == "":
		writeError(w, http.StatusBadRequest, ownerRequiredMessage(p))
		return
	case controlChars.MatchString(req.Name + req.Host + req.OwnerID + req.ServerName + req.WorldName + req.ServerDesc + req.DataPath):
		writeError(w, http.StatusBadRequest, "names, paths and ids cannot contain line breaks or control characters")
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
	slug := slugify(req.Name, p.SlugFallback)
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
		req.GamePort = p.DefaultGamePort
	}
	if req.AgentPort == 0 {
		req.AgentPort = 8811
	}
	for _, ap := range p.AdminPorts {
		if adminPortValue(&req, ap.Key) == 0 {
			setAdminPortValue(&req, ap.Key, ap.Default)
		}
	}
	// The game binds GamePortCount contiguous ports from GamePort, so the
	// whole run has to fit and stay clear of the agent's port.
	maxGamePort := 65536 - p.portCount()
	if req.GamePort < 1 || req.GamePort > maxGamePort {
		msg := fmt.Sprintf("game port must be in 1-%d", maxGamePort)
		if p.portCount() > 1 {
			msg += " — the game also uses the port(s) above it"
		}
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.AgentPort < 1 || req.AgentPort > 65535 {
		writeError(w, http.StatusBadRequest, "agent port must be in 1-65535")
		return
	}
	// Every published port must be distinct: the game's run, the named
	// admin transports, and the agent's port (Palworld's four-way check).
	seen := map[int]string{}
	for i := 0; i < p.portCount(); i++ {
		seen[req.GamePort+i] = "game"
	}
	for _, ap := range p.AdminPorts {
		v := adminPortValue(&req, ap.Key)
		if v < 1 || v > 65535 {
			writeError(w, http.StatusBadRequest, ap.Key+" port must be in 1-65535")
			return
		}
		if prev, dup := seen[v]; dup {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("%s port %d collides with the %s port", ap.Key, v, prev))
			return
		}
		seen[v] = ap.Key
	}
	if prev, dup := seen[req.AgentPort]; dup && prev != "game" {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("agent port %d collides with the %s port", req.AgentPort, prev))
		return
	}
	if req.AgentPort >= req.GamePort && req.AgentPort < req.GamePort+p.portCount() {
		if p.portCount() > 1 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("agent port %d collides with the game's port range (%d-%d)", req.AgentPort, req.GamePort, req.GamePort+p.portCount()-1))
		} else {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("agent port %d collides with the game port", req.AgentPort))
		}
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
	// never seen, leaving a server flametender can see and never reach. (The
	// provisioner refuses this itself — checked here too so an older
	// provisioner image, which only fails at docker create, is covered.)
	if s.Provisioner != nil {
		if found, err := s.Provisioner.Discover(r.Context()); err == nil {
			for _, f := range found {
				if f.Name == p.AgentName+"-"+slug {
					writeError(w, http.StatusConflict, conflictMessage(p, slug))
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
	identityEnv := ""
	if req.OwnerID != "" {
		identityEnv += fmt.Sprintf("      %s_OWNER_ID: %q\n", p.EnvPrefix, req.OwnerID)
	}
	identityEnv += fmt.Sprintf("      %s_SERVER_NAME: %q\n", p.EnvPrefix, req.ServerName)
	if req.ServerDesc != "" {
		identityEnv += fmt.Sprintf("      %s_SERVER_DESC: %q\n", p.EnvPrefix, req.ServerDesc)
	}
	if req.WorldName != "" {
		identityEnv += fmt.Sprintf("      %s_WORLD_NAME: %q\n", p.EnvPrefix, req.WorldName)
	}
	if req.JoinPassword != "" {
		identityEnv += fmt.Sprintf("      %s_JOIN_PASSWORD: %q\n", p.EnvPrefix, req.JoinPassword)
	}

	stack := fmt.Sprintf(`# %s — %s, generated by the console.
# Deploy as its own stack (TrueNAS custom app / docker compose).
%sservices:
  %s:
    image: %s:%s
%s    environment:
      # SteamCMD needs a writable home; the run-as user has none in the image.
      HOME: /tmp
      %s_MODE: supervisor
      %s_TOKEN: %s
      %s_ADMIN_PASSWORD: %s
%s    ports:
%s      - "%d:8811"     # agent API — the dashboard's only channel
    volumes:
      # Must be writable by the container user — uid 1000 unless user:
      # overrides it.
      - %s:%s
    restart: unless-stopped
`, req.Name, p.StackHeadline, p.StackNotes,
		p.AgentName, p.ImageRepo, req.ImageTag, userLine,
		p.EnvPrefix, p.EnvPrefix, token, p.EnvPrefix, req.AdminPassword, identityEnv,
		stackGamePorts(p, &req),
		req.AgentPort, req.DataPath, p.MountPath)

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
		result, err := s.Provisioner.Provision(r.Context(), agent.ProvisionRequest{
			Slug:          slug,
			ImageTag:      req.ImageTag,
			Token:         token,
			AdminPassword: req.AdminPassword,
			JoinPassword:  req.JoinPassword,
			ServerName:    req.ServerName,
			ServerDesc:    req.ServerDesc,
			OwnerID:       req.OwnerID,
			WorldName:     req.WorldName,
			RunAs:         req.RunAs,
			GamePort:      req.GamePort,
			RESTPort:      req.RESTPort,
			RCONPort:      req.RCONPort,
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
			writeError(w, http.StatusConflict, conflictMessage(p, slug))
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
		Name: req.Name, Host: req.Host,
		GamePort:   req.GamePort,
		Enabled:    true,
		AgentURL:   fmt.Sprintf("http://%s:%d", req.Host, req.AgentPort),
		AgentToken: token,
		// Recorded only when the provisioner actually made it: this is the
		// name the destroy path passes back, and — when flametender's own docker
		// proxy happens to watch the same daemon — what the container logs
		// viewer and watchdog key off. Power control is unaffected either
		// way, since every power site tries agentSupervisor before docker.
		ContainerName: container,
	}
	// Games with named admin transports get the row wired for them, so
	// the dashboard can speak REST/RCON the moment the server exists.
	if p.adminPort("rcon") != nil {
		srv.RCONPort, srv.RCONPassword = req.RCONPort, req.AdminPassword
	}
	if p.adminPort("rest") != nil {
		srv.RESTPort, srv.RESTPassword = req.RESTPort, req.AdminPassword
		srv.UseREST = true
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
func conflictMessage(p *ProvisionProfile, slug string) string {
	return fmt.Sprintf("a container named %s-%s already exists on the host — "+
		"pick a different name, or adopt the existing container from Add server", p.AgentName, slug)
}

// handleProvisionDefaults reports everything the wizard can prefill: the
// provisioner's own configuration (data root, public host, run-as, image
// tag) plus a free-port proposal computed from the servers flametender already
// manages. The proposal is a suggestion — something else on the box can
// still hold a port, in which case the deploy fails cleanly at create
// time.
func (s *Server) handleProvisionDefaults(w http.ResponseWriter, r *http.Request) {
	if s.Provisioner == nil || s.Provision == nil {
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

	// Containers hold ports too — including ones whose flametender row was
	// deleted. The provisioner sees them; a proposal that ignored them
	// would suggest ports that fail at deploy time.
	var containerPorts []int
	if found, err := s.Provisioner.Discover(r.Context()); err == nil {
		for _, f := range found {
			containerPorts = append(containerPorts, f.GamePort, f.RESTPort, f.RCONPort, f.AgentPort)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"host":      s.inferHost(health.Provision.PublicHost, servers),
		"dataRoot":  health.Provision.DataRoot,
		"runAs":     health.Provision.RunAs,
		"imageTag":  health.Provision.ImageTag,
		"ports":     proposePorts(s.Provision, servers, containerPorts),
	})
}

// inferHost picks the address for new servers: the provisioner's declared
// public host wins; else the host part of the provisioner URL when it's a
// real address (a bare compose service name — no dots, not an IP — can't
// be reached by players or by flametender's REST client); else the address the
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

// proposePorts finds the first offset where the game's whole port run and
// the agent's port are all free of anything the console tracks or the
// host's containers hold. The run moves together because the game binds
// every port in it.
func proposePorts(p *ProvisionProfile, servers []*store.Server, containerPorts []int) map[string]int {
	count := p.portCount()
	used := map[int]bool{}
	claim := func(port int) {
		if port == 0 {
			return
		}
		for i := 0; i < count; i++ {
			used[port+i] = true
		}
	}
	for _, srv := range servers {
		claim(srv.GamePort)
		if u, err := url.Parse(srv.AgentURL); err == nil {
			if ap, err := strconv.Atoi(u.Port()); err == nil {
				used[ap] = true
			}
		}
	}
	for _, cp := range containerPorts {
		claim(cp)
	}
	for offset := 0; offset < 1000; offset++ {
		gamePort, agentPort := p.DefaultGamePort+(offset*count), 8811+offset
		free := !used[agentPort]
		for i := 0; free && i < count; i++ {
			free = !used[gamePort+i]
		}
		proposal := map[string]int{"game": gamePort, "agent": agentPort}
		for _, ap := range p.AdminPorts {
			v := ap.Default + offset
			if used[v] || v == gamePort || v == agentPort {
				free = false
				break
			}
			proposal[ap.Key] = v
		}
		if free {
			return proposal
		}
	}
	fallback := map[string]int{"game": p.DefaultGamePort, "agent": 8811}
	for _, ap := range p.AdminPorts {
		fallback[ap.Key] = ap.Default
	}
	return fallback
}

// stackGamePorts renders the game's port mapping lines for the stack —
// the UDP run, then the named TCP admin transports.
func stackGamePorts(p *ProvisionProfile, req *provisionRequest) string {
	out := fmt.Sprintf("      - %q   # %s\n", fmt.Sprintf("%d:%d/udp", req.GamePort, p.DefaultGamePort), p.GamePortComment)
	for i := 1; i < p.portCount(); i++ {
		out += fmt.Sprintf("      - %q   # the game's paired port\n", fmt.Sprintf("%d:%d/udp", req.GamePort+i, p.DefaultGamePort+i))
	}
	for _, ap := range p.AdminPorts {
		out += fmt.Sprintf("      - %q   # %s\n", fmt.Sprintf("%d:%d", adminPortValue(req, ap.Key), ap.Container), ap.Comment)
	}
	return out
}

// adminPortValue/setAdminPortValue map the well-known admin keys onto
// the typed request fields — the wire shape the consoles' wizards
// already send.
func adminPortValue(req *provisionRequest, key string) int {
	switch key {
	case "rest":
		return req.RESTPort
	case "rcon":
		return req.RCONPort
	}
	return 0
}

func setAdminPortValue(req *provisionRequest, key string, v int) {
	switch key {
	case "rest":
		req.RESTPort = v
	case "rcon":
		req.RCONPort = v
	}
}

// ownerRequiredMessage explains the refusal, with the game's own pointer
// to where the id lives when the profile supplies one.
func ownerRequiredMessage(p *ProvisionProfile) string {
	msg := "owner id is required — the game will not start without one"
	if p.OwnerIDHelp != "" {
		msg += " (" + p.OwnerIDHelp + ")"
	}
	return msg
}

// handleProvisionDiscover surfaces flameagent containers already on the
// provisioner's host, marking the ones flametender knows about so the add
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
	if s.Provisioner == nil || s.Provision == nil {
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
	// The two halves of reachability fail for unrelated reasons, so name
	// the one that's actually missing — "add it manually" alone sends the
	// operator hunting through the wrong config.
	if adopted.Token == "" {
		writeError(w, http.StatusBadRequest,
			"the provisioner returned no agent token for that container — adopting through Anvil, that usually means this console's client registration is missing its envPrefix (\""+
				s.Provision.EnvPrefix+"_\"), so every variable was filtered out; fix the registration and re-adopt, or add the server manually")
		return
	}
	if adopted.AgentPort == 0 {
		writeError(w, http.StatusBadRequest,
			"that container does not publish the agent port (8811) to the host, so there is no address to reach its agent on — republish the port, or add the server manually")
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
			"could not infer the host address — declare a public host on the provisioner or pass one")
		return
	}

	name := adopted.ServerName
	if name == "" {
		name = strings.ReplaceAll(strings.TrimPrefix(adopted.Name, s.Provision.AgentName+"-"), "-", " ")
	}
	srv := &store.Server{
		Name: name, Host: host,
		GamePort:      adopted.GamePort,
		Enabled:       true,
		AgentURL:      fmt.Sprintf("http://%s:%d", host, adopted.AgentPort),
		AgentToken:    adopted.Token,
		ContainerName: adopted.Name,
	}
	if s.Provision.adminPort("rcon") != nil && adopted.RCONPort != 0 {
		srv.RCONPort, srv.RCONPassword = adopted.RCONPort, adopted.AdminPassword
	}
	if s.Provision.adminPort("rest") != nil && adopted.RESTPort != 0 {
		srv.RESTPort, srv.RESTPassword = adopted.RESTPort, adopted.AdminPassword
		srv.UseREST = true
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
func slugify(name, fallback string) string {
	slug := strings.ToLower(name)
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = fallback
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return strings.Trim(slug, "-")
}
