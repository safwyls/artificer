package palagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/safwyls/palcon/internal/dockerctl"
)

// Provisioner mode (docs/sidecar-agent.md phase 5): the one component in
// the system allowed to hold docker create rights, exposing exactly one
// verb — instantiate the locked Palworld supervisor template. The
// template lives here in code: a compromised palcon (or leaked
// provisioner token) can stamp out more Palworld servers under the
// configured data root, and nothing else — no arbitrary images, mounts,
// or privileges are expressible through this API.

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
var provRunAsPattern = regexp.MustCompile(`^\d{1,7}:\d{1,7}$`)

// ProvisionRequest instantiates the template. Every field is data for
// the fixed template — none of it changes what kind of container is made.
type ProvisionRequest struct {
	// Slug names the container (palagent-<slug>) and the data directory
	// (<data root>/<slug>).
	Slug string `json:"slug"`
	// ImageTag selects the palagent channel for the new server.
	ImageTag string `json:"imageTag"`
	// Token is the new agent's bearer token (palcon generated it).
	Token string `json:"token"`
	// AdminPassword wires the game's REST/RCON (PALAGENT_ADMIN_PASSWORD).
	AdminPassword string `json:"adminPassword"`
	ServerName    string `json:"serverName"`
	ServerDesc    string `json:"serverDesc"`
	// RunAs is uid:gid for the container ("" = image default/root).
	RunAs string `json:"runAs"`
	// Published host ports.
	GamePort  int `json:"gamePort"`
	RESTPort  int `json:"restPort"`
	RCONPort  int `json:"rconPort"`
	AgentPort int `json:"agentPort"`
}

func (a *Agent) handleProvision(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Mode != "provisioner" {
		writeError(w, http.StatusBadRequest, "agent is not a provisioner")
		return
	}
	var req ProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch {
	case !slugPattern.MatchString(req.Slug):
		writeError(w, http.StatusBadRequest, "slug must be lowercase letters, digits and dashes")
		return
	case len(req.Token) < minTokenLen:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("token must be at least %d characters", minTokenLen))
		return
	case req.AdminPassword == "":
		writeError(w, http.StatusBadRequest, "admin password is required")
		return
	case req.RunAs != "" && !provRunAsPattern.MatchString(req.RunAs):
		writeError(w, http.StatusBadRequest, "runAs must be numeric uid:gid")
		return
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	seen := map[int]bool{}
	for _, p := range []int{req.GamePort, req.RESTPort, req.RCONPort, req.AgentPort} {
		if p < 1 || p > 65535 || seen[p] {
			writeError(w, http.StatusBadRequest, "ports must be distinct and in 1-65535")
			return
		}
		seen[p] = true
	}

	// The data directory is always DataRoot/<slug> — the slug pattern
	// forbids traversal, and nothing else about the location is
	// caller-controlled.
	dataDir := filepath.Join(a.cfg.DataRoot, req.Slug)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "creating data dir: "+err.Error())
		return
	}
	if req.RunAs != "" {
		parts := strings.SplitN(req.RunAs, ":", 2)
		uid, _ := strconv.Atoi(parts[0])
		gid, _ := strconv.Atoi(parts[1])
		if err := os.Chown(dataDir, uid, gid); err != nil {
			// Non-fatal only if the dir is already writable by the user;
			// SteamCMD will tell on it loudly otherwise.
			a.cfg.Logger.Warn("could not chown data dir", "dir", dataDir, "error", err)
		}
	}

	image := "ghcr.io/safwyls/palagent:" + req.ImageTag
	if err := a.docker.ImagePull(r.Context(), image); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	env := []string{
		"HOME=/tmp",
		"PALAGENT_MODE=supervisor",
		"PALAGENT_TOKEN=" + req.Token,
		"PALAGENT_ADMIN_PASSWORD=" + req.AdminPassword,
	}
	if req.ServerName != "" {
		env = append(env, "PALAGENT_SERVER_NAME="+req.ServerName)
	}
	if req.ServerDesc != "" {
		env = append(env, "PALAGENT_SERVER_DESC="+req.ServerDesc)
	}
	name := "palagent-" + req.Slug
	id, err := a.docker.ContainerCreate(r.Context(), dockerctl.ContainerSpec{
		Name:  name,
		Image: image,
		User:  req.RunAs,
		Env:   env,
		Binds: []string{dataDir + ":/palworld"},
		Ports: map[int]string{
			req.GamePort:  "8211/udp",
			req.RESTPort:  "8212/tcp",
			req.RCONPort:  "25575/tcp",
			req.AgentPort: "8811/tcp",
		},
		RestartUnlessStopped: true,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := a.docker.Start(r.Context(), name); err != nil {
		writeError(w, http.StatusBadGateway, "created but failed to start: "+err.Error())
		return
	}
	a.cfg.Logger.Info("provisioned server", "container", name, "dataDir", dataDir, "image", image)
	writeJSON(w, http.StatusCreated, map[string]any{
		"container": name,
		"id":        id,
		"dataDir":   dataDir,
	})
}

// validateProvisionerConfig is called from New for mode=provisioner.
func validateProvisionerConfig(cfg *Config) (*dockerctl.Client, error) {
	if cfg.DataRoot == "" {
		return nil, errors.New("provisioner mode requires a data root")
	}
	if !filepath.IsAbs(cfg.DataRoot) {
		return nil, errors.New("data root must be absolute")
	}
	if cfg.DockerHost == "" {
		cfg.DockerHost = "unix:///var/run/docker.sock"
	}
	return dockerctl.New(cfg.DockerHost)
}
