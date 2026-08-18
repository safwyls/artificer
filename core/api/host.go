package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/safwyls/artificer/core/anvilclient"
)

// The host dashboard: what Anvil manages on the machine this console
// deploys to — every Anvil-placed container (this console's and the other
// consoles'), and the images behind them. The consoles deliberately cannot
// see the Docker host themselves, which keeps the placement rights in one
// place but makes the machine invisible to the operator; this endpoint is
// the read-only window back in.
//
// Deliberately scoped to Anvil's own stack. The host-wide view exists —
// the wizard reads every published port so its proposals cannot collide —
// but it exists for port and name decisions inside the console, not for
// display: on a shared box (a NAS running dozens of unrelated apps) a
// game console has no business relaying the whole machine to a browser.

// fleetSource is what the dashboard needs from the placement service,
// beyond the wizard's Provisioner verbs: Anvil's own wire shapes,
// untranslated. A Provisioner that does not implement it (a test fake)
// simply reports the dashboard unavailable — same as no Anvil at all.
type fleetSource interface {
	FleetHealth(ctx context.Context) (*anvilclient.Health, error)
	FleetContainers(ctx context.Context) ([]anvilclient.ManagedContainer, error)
	FleetImages(ctx context.Context) ([]anvilclient.HostImage, error)
}

// hostContainerView is one fleet row, joined against this console's own
// server registrations: a Mine row whose container name matches a server
// row gets that server's id and name, so the dashboard can link to the
// server page — and so a Mine row *without* a match is visibly an orphan
// (placed by this console, registered nowhere; the adopt flow's job).
type hostContainerView struct {
	anvilclient.ManagedContainer
	ServerID   int64  `json:"serverId,omitempty"`
	ServerName string `json:"serverName,omitempty"`
}

type hostOverview struct {
	// Available=false means no Anvil is wired at all; Reason says so. The
	// per-section errors below are the different case where Anvil is wired
	// but a read failed.
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	AnvilURL  string `json:"anvilURL,omitempty"`
	// Health is this console's own registration view (data root, image
	// allowlist, dockerOk); nil with HealthError set when unreachable.
	Health      *anvilclient.Health     `json:"health,omitempty"`
	HealthError string                  `json:"healthError,omitempty"`
	Containers  []hostContainerView     `json:"containers,omitempty"`
	FleetError  string                  `json:"fleetError,omitempty"`
	Images      []anvilclient.HostImage `json:"images,omitempty"`
	ImagesError string                  `json:"imagesError,omitempty"`
}

// handleHostOverview answers GET /api/host. Admin-only: the rows carry
// data directories, ownership labels and everything the machine publishes,
// which is infrastructure, not player-facing state. Read-only by design —
// every mutation on a container goes through the flows that own it (the
// wizard, the server page's power and destroy verbs), so the dashboard
// cannot grow into a second, unaudited path to the Docker socket.
func (s *Server) handleHostOverview(w http.ResponseWriter, r *http.Request) {
	// A nil Provisioner fails the assertion too, so one check covers both
	// "no Anvil configured" and "a placement service with no fleet view".
	fleet, ok := s.Provisioner.(fleetSource)
	if !ok {
		writeJSON(w, http.StatusOK, hostOverview{
			Available: false,
			Reason:    "this console is not connected to an Anvil host service — set ANVIL_URL and ANVIL_TOKEN to see the host",
		})
		return
	}
	ctx := r.Context()
	out := hostOverview{Available: true, AnvilURL: s.Provisioner.BaseURL()}

	// Each section reports its own failure rather than the first one
	// aborting the response: "health is fine but the container list
	// failed" and "anvil is down entirely" look identical as a bare 502,
	// and the difference is exactly what an operator debugging the host
	// needs.
	health, err := fleet.FleetHealth(ctx)
	if err != nil {
		out.HealthError = err.Error()
	} else {
		out.Health = health
	}

	containers, err := fleet.FleetContainers(ctx)
	if err != nil {
		out.FleetError = err.Error()
	} else {
		byContainer := map[string]struct {
			id   int64
			name string
		}{}
		if servers, err := s.store.ListServers(ctx); err == nil {
			for _, srv := range servers {
				if srv.ContainerName != "" {
					byContainer[srv.ContainerName] = struct {
						id   int64
						name string
					}{srv.ID, srv.Name}
				}
			}
		}
		out.Containers = make([]hostContainerView, 0, len(containers))
		for _, c := range containers {
			// An Anvil from before the ?managed=1 filter answers with every
			// container on the box; the scoping must hold here regardless.
			if !c.Managed {
				continue
			}
			row := hostContainerView{ManagedContainer: c}
			if reg, found := byContainer[c.Name]; found {
				row.ServerID = reg.id
				row.ServerName = reg.name
			}
			out.Containers = append(out.Containers, row)
		}
	}

	images, err := fleet.FleetImages(ctx)
	switch {
	case errors.Is(err, anvilclient.ErrNotFound):
		// An Anvil from before /v1/images. The rest of the dashboard works
		// against it, so say what is missing instead of failing the page.
		out.ImagesError = "this Anvil does not report images yet — upgrade it to ghcr.io/safwyls/anvil:latest to see the host's disk"
	case err != nil:
		out.ImagesError = err.Error()
	default:
		out.Images = images
	}

	writeJSON(w, http.StatusOK, out)
}
