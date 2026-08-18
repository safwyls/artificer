// Package host is Anvil: one service per machine that places, inspects and
// rebuilds the containers a game console asks for.
//
// # Why this exists as its own thing
//
// It started as "provisioner mode" inside a game console's sidecar agent,
// and was copied wholesale into a second console for a second game. Two
// processes then held the same Docker socket on the same machine, created
// containers the same way, and could not see each other — which is not a
// theoretical problem: one console proposed a host port the other was
// already using, the create succeeded, the start failed, and an operator
// was left removing a half-made container by hand.
//
// A host has one Docker socket, so it should have one thing holding it.
//
// # What it knows, and what it refuses to know
//
// Anvil knows about containers, images, ports, paths and disk. It does not
// know what a "world name" is, which game a container runs, or what its
// settings mean — all of that arrives as data in a ProvisionSpec and is
// passed through untouched. That is the whole design: a console owns its
// game's knowledge, and a game's changes never become a deploy of this
// service.
//
// # The security posture this inherits, and must keep
//
// The consoles deliberately hold no Docker rights; this service is the only
// component that does, and it exposes a fixed set of shaped verbs rather
// than anything resembling a generic Docker proxy. A caller cannot name a
// host path, cannot run an image outside the allowlist, and cannot forge
// the ownership labels every destroy and rebuild checks. Those three
// constraints are what keep "place a container for me" from being "run
// anything you like on my NAS", and none of them should be relaxed without
// treating it as a change to the host's security posture.
package host

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/anvil/internal/dockerctl"
)

// APIVersion lets a console refuse to drive a service it doesn't
// understand, rather than failing in some more interesting way later.
const APIVersion = 1

// minTokenLen is the floor for every client token. The service refuses to
// start below it rather than run guessably authenticated — it holds the
// Docker socket, so "someone guessed the token" is not a small event.
const minTokenLen = 16

// Ownership labels. Every container Anvil makes carries these, and every
// destroy or rebuild checks them before touching anything.
const (
	LabelManaged = "anvil.managed"
	LabelSlug    = "anvil.slug"
	// LabelOwner records which console placed a container. Enforced, not
	// advisory: the owner is taken from the caller's token, never from the
	// request, and every destroy and rebuild requires it to match.
	LabelOwner = "anvil.owner"
)

// legacyManagedLabels are the per-console labels that existed before this
// service did. Containers carrying them were made by a console's own
// built-in provisioner and are still ours to manage — recognising them is
// what lets an existing deployment move to Anvil without relabelling live
// containers or, worse, orphaning them. The label also names the console
// that made the container, so ownership survives the migration too.
var legacyManagedLabels = []string{"wildskeeper.provisioned", "palcon.provisioned"}

// legacySlugLabels and legacyOwners mirror legacyManagedLabels, index for
// index.
var (
	legacySlugLabels = []string{"wildskeeper.slug", "palcon.slug"}
	legacyOwners     = []string{"wildskeeper", "palcon"}
)

type Config struct {
	// Clients are the registered consoles, each with its own token, data
	// root and image allowlist. At least one is required — there is no
	// shared-token mode, because a shared token cannot express ownership.
	Clients []ClientConfig
	// DockerHost is the Docker endpoint. Required.
	DockerHost string
	// PublicHost is the address consoles and players reach this machine on.
	// Reported to consoles so a wizard can prefill instead of asking; inside
	// a container "localhost" means the container, so it must be declared.
	PublicHost string
	// AllowedImagePrefixes is the fallback allowlist for clients that don't
	// bring their own. Empty means DefaultImagePrefixes.
	AllowedImagePrefixes []string
	// DefaultRunAs is the uid:gid suggested to consoles that don't specify.
	DefaultRunAs string
	Version      string
	Logger       *slog.Logger
}

type Service struct {
	cfg     Config
	clients []client
	docker  *dockerctl.Client
}

func New(cfg Config) (*Service, error) {
	if len(cfg.Clients) == 0 {
		return nil, errors.New("at least one client must be registered (ANVIL_CLIENTS / ANVIL_CLIENTS_FILE)")
	}
	if cfg.DockerHost == "" {
		return nil, errors.New("docker host is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	seenID := map[string]bool{}
	seenHash := map[[32]byte]bool{}
	clients := make([]client, 0, len(cfg.Clients))
	for _, cc := range cfg.Clients {
		if err := cc.validate(); err != nil {
			return nil, err
		}
		if seenID[cc.ID] {
			return nil, fmt.Errorf("client id %q registered twice", cc.ID)
		}
		seenID[cc.ID] = true
		hash := sha256.Sum256([]byte(cc.Token))
		// Two consoles sharing a token would make resolve ambiguous — the
		// last match would silently win, which is worse than refusing.
		if seenHash[hash] {
			return nil, fmt.Errorf("client %q shares a token with another client; each console needs its own", cc.ID)
		}
		seenHash[hash] = true
		clients = append(clients, client{
			ID: cc.ID, tokenHash: hash, DataRoot: cc.DataRoot,
			ImagePrefixes: cc.ImagePrefixes, EnvPrefix: cc.EnvPrefix,
		})
	}
	docker, err := dockerctl.New(cfg.DockerHost)
	if err != nil {
		return nil, fmt.Errorf("docker endpoint: %w", err)
	}
	return &Service{cfg: cfg, clients: clients, docker: docker}, nil
}

func (s *Service) Handler() http.Handler {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.requireToken)
		r.Get("/health", s.handleHealth)

		// Placement and lifecycle. Every one of these is shaped: there is
		// no verb here that takes an arbitrary path or an arbitrary command.
		r.Post("/provision", s.handleProvision)
		r.Post("/provision/recreate", s.handleRecreate)
		r.Post("/provision/destroy", s.handleDestroy)

		// Recovery: find and re-register servers the console lost the row
		// for. Both are scoped to what the caller could have deployed.
		r.Get("/discover", s.handleDiscover)
		r.Post("/adopt", s.handleAdopt)

		// The fleet view. Read-only, and the reason a second console can
		// stop colliding with the first.
		r.Get("/containers", s.handleListContainers)
		r.Get("/ports", s.handlePorts)
	})
	return r
}

// requireToken identifies the calling console from its bearer token and
// stashes it in the request context. Bearer-token-only on purpose: this
// service is reached over a LAN by machines, not people, and a
// session/cookie scheme would be more surface for no gain. Which console a
// token belongs to is what turns the owner label from a comment into a
// rule — every ownership check downstream reads the identity established
// here.
func (s *Service) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := s.resolve(r)
		if c == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, c)))
	})
}

type Health struct {
	Service    string `json:"service"`
	Version    string `json:"version"`
	APIVersion int    `json:"apiVersion"`
	// Client is who this answer is for — the console the presented token
	// belongs to. A console can use it to detect a misconfigured token
	// (wildskeeper holding palcon's) before anything is placed.
	Client string `json:"client"`
	// DataRoot is the calling console's own root, not anyone else's.
	DataRoot   string `json:"dataRoot"`
	PublicHost string `json:"publicHost,omitempty"`
	RunAs      string `json:"runAs,omitempty"`
	// AllowedImagePrefixes is reported so a console can explain a refusal
	// before making one, rather than only after.
	AllowedImagePrefixes []string `json:"allowedImagePrefixes"`
	// DockerOk reports whether the socket answered. False here is the
	// difference between "this service is broken" and "your request was".
	DockerOk bool `json:"dockerOk"`
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	c := caller(r)
	_, err := s.docker.ContainerList(r.Context())
	writeJSON(w, http.StatusOK, Health{
		Service:              "anvil",
		Version:              s.cfg.Version,
		APIVersion:           APIVersion,
		Client:               c.ID,
		DataRoot:             c.DataRoot,
		PublicHost:           s.cfg.PublicHost,
		RunAs:                s.cfg.DefaultRunAs,
		AllowedImagePrefixes: s.allowlistFor(c),
		DockerOk:             err == nil,
	})
}

// allowlistFor is the image allowlist that applies to one console: its own
// when registered with one, the service-wide fallback otherwise.
func (s *Service) allowlistFor(c *client) []string {
	if len(c.ImagePrefixes) > 0 {
		return c.ImagePrefixes
	}
	if len(s.cfg.AllowedImagePrefixes) > 0 {
		return s.cfg.AllowedImagePrefixes
	}
	return DefaultImagePrefixes
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Conflict reasons. A 409 from /v1/provision has two causes and the right
// thing to tell an operator differs between them — "pick another name, or
// adopt the container that's already there" is useless advice for a port
// collision. The prose says which, but a console should not have to parse
// prose to route a message, so the reason is a field. Additive on purpose:
// a client that doesn't know these still gets the same status and the same
// sentence.
const (
	ReasonNameTaken  = "name-taken"
	ReasonPortsInUse = "ports-in-use"
)

func writeConflict(w http.ResponseWriter, reason, msg string) {
	writeJSON(w, http.StatusConflict, map[string]string{"error": msg, "reason": reason})
}
