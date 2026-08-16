package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/safwyls/ilmari/internal/dockerctl"
)

// placeError carries the status a failed placement should answer with, so
// callers can report which step failed rather than collapsing everything
// into one gateway error.
type placeError struct {
	status int
	err    error
}

func (e *placeError) Error() string { return e.err.Error() }

func writePlaceError(w http.ResponseWriter, err error) {
	var pe *placeError
	if errors.As(err, &pe) {
		writeError(w, pe.status, pe.err.Error())
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

// place creates and starts one container from a spec. Every provision comes
// through here — there is deliberately no second path, because a second one
// would mean a second set of ownership labels, chown rules and failure
// semantics to keep in step.
//
// Everything caller-dependent — the data root the slug lands under, the
// image allowlist, the owner label — comes from the resolved client, never
// from the request. That is the enforcement half of per-console tokens.
func (s *Service) place(ctx context.Context, spec ProvisionSpec, c *client) (string, error) {
	if err := spec.Validate(s.allowlistFor(c)); err != nil {
		return "", &placeError{http.StatusBadRequest, err}
	}

	// The data directory is always <caller's data root>/<slug>. The slug
	// pattern forbids traversal and nothing else about the location is
	// caller-controlled — this is the constraint that keeps a spec from
	// being able to mount anything it likes.
	dataDir := filepath.Join(c.DataRoot, spec.Slug)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", &placeError{http.StatusInternalServerError, fmt.Errorf("creating data dir: %w", err)}
	}
	if spec.User != "" {
		parts := strings.SplitN(spec.User, ":", 2)
		uid, _ := strconv.Atoi(parts[0])
		gid := uid
		if len(parts) == 2 {
			gid, _ = strconv.Atoi(parts[1])
		}
		if err := os.Chown(dataDir, uid, gid); err != nil {
			// Not fatal on its own: the directory may already be writable by
			// that user. Whatever runs inside will complain far more
			// usefully than a guess here could.
			s.cfg.Logger.Warn("could not chown data dir", "dir", dataDir, "error", err)
		}
	}

	if err := s.docker.ImagePull(ctx, spec.Image); err != nil {
		return "", &placeError{http.StatusBadGateway, err}
	}

	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env) // stable, so a later rebuild diffs cleanly
	ports := map[int]string{}
	for _, p := range spec.Ports {
		proto := p.Proto
		if proto == "" {
			proto = "tcp"
		}
		ports[p.Host] = fmt.Sprintf("%d/%s", p.Container, proto)
	}
	mount := spec.DataMount
	if mount == "" {
		mount = "/data"
	}

	// Ownership labels go on last so a caller cannot overwrite them. They
	// are what every destroy and rebuild reads, so a forged one would let a
	// console claim a container this service never made.
	labels := map[string]string{}
	for k, v := range spec.Labels {
		labels[k] = v
	}
	labels[LabelManaged] = "true"
	labels[LabelSlug] = spec.Slug
	labels[LabelOwner] = c.ID

	if _, err := s.docker.ContainerCreate(ctx, dockerctl.ContainerSpec{
		Name:                 spec.Name,
		Image:                spec.Image,
		User:                 spec.User,
		Env:                  env,
		Binds:                []string{dataDir + ":" + mount},
		Ports:                ports,
		Labels:               labels,
		RestartUnlessStopped: true,
	}); err != nil {
		return "", &placeError{http.StatusBadGateway, err}
	}
	if err := s.docker.Start(ctx, spec.Name); err != nil {
		return "", &placeError{http.StatusBadGateway, fmt.Errorf("created but failed to start: %w", err)}
	}
	return dataDir, nil
}

func (s *Service) handleProvision(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	// Note there is no owner field in the request: who is asking comes from
	// the token, and a console cannot place containers as anyone else.
	var req ProvisionSpec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Name and port conflicts are checked before anything is created, and
	// answered as 409 — "nothing was made" rather than "something went
	// wrong partway through". This is the check that did not exist when two
	// provisioners shared a host: one proposed a port the other held, the
	// create succeeded, the start failed, and the leftover had to be
	// removed by hand.
	containers, err := s.docker.ContainerList(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "listing containers: "+err.Error())
		return
	}
	for _, c := range containers {
		if c.Name == req.Name {
			writeError(w, http.StatusConflict, "a container named "+req.Name+" already exists on this host")
			return
		}
	}
	if taken := conflictingPorts(containers, req.Ports); len(taken) > 0 {
		writeError(w, http.StatusConflict, "host "+plural(len(taken), "port", "ports")+" already in use: "+strings.Join(taken, ", "))
		return
	}

	dataDir, err := s.place(r.Context(), req, c)
	if err != nil {
		writePlaceError(w, err)
		return
	}
	s.cfg.Logger.Info("placed container", "container", req.Name, "image", req.Image, "owner", c.ID, "dataDir", dataDir)
	writeJSON(w, http.StatusCreated, map[string]any{
		"container": req.Name,
		"dataDir":   dataDir,
		"image":     req.Image,
	})
}

// conflictingPorts reports which requested host ports are already published
// on this machine, named with what holds them so the answer is actionable.
func conflictingPorts(containers []dockerctl.ContainerSummary, want []PortMap) []string {
	held := map[int]string{}
	for _, c := range containers {
		for _, hostPort := range c.Ports {
			if hostPort != 0 {
				held[hostPort] = c.Name
			}
		}
	}
	var out []string
	for _, p := range want {
		if by, ok := held[p.Host]; ok {
			out = append(out, fmt.Sprintf("%d (%s)", p.Host, by))
		}
	}
	sort.Strings(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type recreateRequest struct {
	Container string `json:"container"`
	Image     string `json:"image"`
}

// handleRecreate rebuilds a managed container on a different image, keeping
// everything else.
//
// Docker has no "change the image" operation, only create, so swapping one
// means removing and rebuilding — easy to do destructively and hard to do
// faithfully. This service made these containers and can read their
// configuration back, which is what makes it a request instead of a
// runbook: nothing else on the host manages them, so without this an
// operator is hand-writing docker on the machine.
func (s *Service) handleRecreate(w http.ResponseWriter, r *http.Request) {
	var req recreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c := caller(r)
	if err := checkImage(req.Image, s.allowlistFor(c)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	found, err := s.findOwned(r.Context(), req.Container, c, w)
	if err != nil || found == nil {
		return
	}

	spec, err := s.docker.InspectSpec(r.Context(), found.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading the container's configuration: "+err.Error())
		return
	}
	previous := spec.Image
	if previous == req.Image {
		writeJSON(w, http.StatusOK, map[string]any{"container": req.Container, "image": req.Image, "previousImage": previous})
		return
	}

	// Pull before removing anything: an image that doesn't exist must fail
	// while the old container is still running, not after it's gone.
	if err := s.docker.ImagePull(r.Context(), req.Image); err != nil {
		writeError(w, http.StatusBadGateway, "pulling "+req.Image+": "+err.Error())
		return
	}
	spec.Image = req.Image

	// Stop first, so whatever is inside gets its grace period to shut down
	// cleanly rather than being removed out from under itself.
	if err := s.docker.Stop(r.Context(), found.ID); err != nil {
		s.cfg.Logger.Warn("stop before rebuild failed; attempting remove anyway", "container", req.Container, "error", err)
	}
	if err := s.docker.ContainerRemove(r.Context(), found.ID); err != nil {
		writeError(w, http.StatusBadGateway, "removing the old container: "+err.Error())
		return
	}
	if _, err := s.docker.ContainerCreate(r.Context(), *spec); err != nil {
		// The old container is already gone. Say what state the host is in
		// rather than leaving it to be discovered.
		s.cfg.Logger.Error("rebuild failed after removing the old container", "container", req.Container, "error", err)
		writeError(w, http.StatusBadGateway,
			"the old container was removed but the new one could not be created ("+err.Error()+"); data in the data directory is untouched")
		return
	}
	if err := s.docker.Start(r.Context(), req.Container); err != nil {
		writeError(w, http.StatusBadGateway, "rebuilt but failed to start: "+err.Error())
		return
	}
	s.cfg.Logger.Info("rebuilt container", "container", req.Container, "from", previous, "to", req.Image)
	writeJSON(w, http.StatusOK, map[string]any{"container": req.Container, "image": req.Image, "previousImage": previous})
}

func (s *Service) handleDestroy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Container string `json:"container"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c := caller(r)
	found, err := s.findOwned(r.Context(), req.Container, c, w)
	if err != nil || found == nil {
		return
	}
	_, slug, _ := ownerOf(found.Labels)

	// Stop first so whatever is inside can flush; the data it leaves behind
	// is the whole reason the directory is kept.
	if err := s.docker.Stop(r.Context(), found.ID); err != nil {
		s.cfg.Logger.Warn("stop before destroy failed; attempting remove anyway", "container", req.Container, "error", err)
	}
	if err := s.docker.ContainerRemove(r.Context(), found.ID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	dataDir := ""
	if slug != "" {
		dataDir = filepath.Join(c.DataRoot, slug)
	}
	// The data directory is deliberately kept. Unmaking a container is not
	// consent to delete what it was holding.
	s.cfg.Logger.Info("destroyed container", "container", req.Container, "dataKept", dataDir)
	writeJSON(w, http.StatusOK, map[string]any{"container": req.Container, "dataDir": dataDir})
}

// findOwned resolves a container name to something *this caller* may act
// on, writing the refusal itself when it may not. A nil result with a nil
// error means the response has already been written.
//
// The two refusals are deliberately different messages: "not created by
// Ilmari" points at wherever the container is actually managed, while
// "belongs to a different console" names the real problem without letting
// one console act across the boundary. What a foreign container never gets
// is silently treated as fair game.
func (s *Service) findOwned(ctx context.Context, name string, c *client, w http.ResponseWriter) (*dockerctl.ContainerSummary, error) {
	containers, err := s.docker.ContainerList(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return nil, err
	}
	for i := range containers {
		if containers[i].Name != name {
			continue
		}
		owner, _, ok := ownerOf(containers[i].Labels)
		if !ok {
			writeError(w, http.StatusBadRequest,
				"that container was not created by Ilmari — manage it wherever it was deployed")
			return nil, nil
		}
		if owner != c.ID {
			writeError(w, http.StatusForbidden, errForeign.Error())
			return nil, nil
		}
		return &containers[i], nil
	}
	writeError(w, http.StatusNotFound, "no container with that name")
	return nil, nil
}

// ManagedContainer is the fleet view's row.
type ManagedContainer struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Running bool   `json:"running"`
	// Managed reports whether Ilmari made it; Mine whether the calling
	// console may act on it. Unmanaged and foreign containers are still
	// listed — they hold ports and disk, and leaving them out is how a
	// console ends up proposing a port something else already has — but
	// only Mine rows carry a slug and data directory.
	Managed bool      `json:"managed"`
	Mine    bool      `json:"mine"`
	Slug    string    `json:"slug,omitempty"`
	Owner   string    `json:"owner,omitempty"`
	Ports   []PortMap `json:"ports,omitempty"`
	DataDir string    `json:"dataDir,omitempty"`
}

// handleListContainers reports every container on the host, ours or not.
//
// Deliberately everything: the reason this service exists is that two
// consoles could not see past their own containers, and a view that only
// showed Ilmari's would reproduce exactly that blindness one level up.
// Nothing about a container's configuration is included — env carries
// tokens and passwords, and a fleet view is not worth leaking them for.
// (Adopt is the one deliberate exception, scoped to the caller's own env
// namespace; its justification lives on AdoptResult.)
func (s *Service) handleListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.docker.ContainerList(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	me := caller(r)
	out := make([]ManagedContainer, 0, len(containers))
	for _, c := range containers {
		owner, slug, ok := ownerOf(c.Labels)
		row := ManagedContainer{
			Name: c.Name, Image: c.Image, Running: c.State == "running",
			Managed: ok, Owner: owner,
			// Mine marks the rows this console may act on; everything else
			// is context for port and name decisions, nothing more.
			Mine: ok && owner == me.ID,
		}
		// Slug and data directory are the caller's own business only. A
		// foreign row shows enough to explain a port collision and no more.
		if row.Mine {
			row.Slug = slug
			if slug != "" {
				row.DataDir = filepath.Join(me.DataRoot, slug)
			}
		}
		for containerSide, hostPort := range c.Ports {
			if hostPort == 0 {
				continue
			}
			port, proto := splitPortSpec(containerSide)
			row.Ports = append(row.Ports, PortMap{Host: hostPort, Container: port, Proto: proto})
		}
		sort.Slice(row.Ports, func(i, j int) bool { return row.Ports[i].Host < row.Ports[j].Host })
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"containers": out})
}

// handlePorts reports every published host port and what holds it, so a
// console can propose a free one instead of discovering a collision at
// start. This is the smallest useful thing one shared service does that two
// separate ones structurally could not.
func (s *Service) handlePorts(w http.ResponseWriter, r *http.Request) {
	containers, err := s.docker.ContainerList(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	type taken struct {
		Port      int    `json:"port"`
		Proto     string `json:"proto"`
		Container string `json:"container"`
	}
	var out []taken
	for _, c := range containers {
		for containerSide, hostPort := range c.Ports {
			if hostPort == 0 {
				continue
			}
			_, proto := splitPortSpec(containerSide)
			out = append(out, taken{Port: hostPort, Proto: proto, Container: c.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	writeJSON(w, http.StatusOK, map[string]any{"ports": out})
}

// splitPortSpec parses docker's "7777/udp" container-side port notation.
func splitPortSpec(spec string) (int, string) {
	proto := "tcp"
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		proto = spec[i+1:]
		spec = spec[:i]
	}
	port, _ := strconv.Atoi(spec)
	return port, proto
}
