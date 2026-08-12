// Package host is Ilmari: one service per machine that places, inspects and
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
// Ilmari knows about containers, images, ports, paths and disk. It does not
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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/ilmari/internal/dockerctl"
)

// APIVersion lets a console refuse to drive a service it doesn't
// understand, rather than failing in some more interesting way later.
const APIVersion = 1

// minTokenLen is the floor for the shared token. The service refuses to
// start below it rather than run guessably authenticated — it holds the
// Docker socket, so "someone guessed the token" is not a small event.
const minTokenLen = 16

// Ownership labels. Every container Ilmari makes carries these, and every
// destroy or rebuild checks them before touching anything.
const (
	LabelManaged = "ilmari.managed"
	LabelSlug    = "ilmari.slug"
	// LabelOwner records which console asked, so a fleet view can group by
	// it and a console can recognise its own. Advisory only: it is not a
	// permission check, because a shared token cannot be one.
	LabelOwner = "ilmari.owner"
)

// legacyManagedLabels are the per-console labels that existed before this
// service did. Containers carrying them were made by a console's own
// built-in provisioner and are still ours to manage — recognising them is
// what lets an existing deployment move to Ilmari without relabelling live
// containers or, worse, orphaning them.
var legacyManagedLabels = []string{"wildskeeper.provisioned", "palcon.provisioned"}

// legacySlugLabels mirror legacyManagedLabels for the slug.
var legacySlugLabels = []string{"wildskeeper.slug", "palcon.slug"}

type Config struct {
	// Token is the shared bearer token consoles present. Required.
	Token string
	// DockerHost is the Docker endpoint. Required.
	DockerHost string
	// DataRoot is the directory per-server data directories are created
	// under. Required, and the only place caller-named slugs can land.
	DataRoot string
	// PublicHost is the address consoles and players reach this machine on.
	// Reported to consoles so a wizard can prefill instead of asking; inside
	// a container "localhost" means the container, so it must be declared.
	PublicHost string
	// AllowedImagePrefixes bounds what this host can be told to run. Empty
	// means DefaultImagePrefixes.
	AllowedImagePrefixes []string
	// DefaultRunAs is the uid:gid suggested to consoles that don't specify.
	DefaultRunAs string
	Version      string
	Logger       *slog.Logger
}

type Service struct {
	cfg    Config
	docker *dockerctl.Client
}

func New(cfg Config) (*Service, error) {
	if len(cfg.Token) < minTokenLen {
		return nil, fmt.Errorf("token must be at least %d characters", minTokenLen)
	}
	if cfg.DockerHost == "" {
		return nil, errors.New("docker host is required")
	}
	if cfg.DataRoot == "" {
		return nil, errors.New("data root is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	docker, err := dockerctl.New(cfg.DockerHost)
	if err != nil {
		return nil, fmt.Errorf("docker endpoint: %w", err)
	}
	return &Service{cfg: cfg, docker: docker}, nil
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

		// The fleet view. Read-only, and the reason a second console can
		// stop colliding with the first.
		r.Get("/containers", s.handleListContainers)
		r.Get("/ports", s.handlePorts)
	})
	return r
}

// requireToken is a constant-time bearer check. Deliberately the only
// authentication: this service is reached over a LAN by machines, not
// people, and a session/cookie scheme would be more surface for no gain.
func (s *Service) requireToken(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(s.cfg.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type Health struct {
	Service    string `json:"service"`
	Version    string `json:"version"`
	APIVersion int    `json:"apiVersion"`
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
	_, err := s.docker.ContainerList(r.Context())
	allowed := s.cfg.AllowedImagePrefixes
	if len(allowed) == 0 {
		allowed = DefaultImagePrefixes
	}
	writeJSON(w, http.StatusOK, Health{
		Service:              "ilmari",
		Version:              s.cfg.Version,
		APIVersion:           APIVersion,
		DataRoot:             s.cfg.DataRoot,
		PublicHost:           s.cfg.PublicHost,
		RunAs:                s.cfg.DefaultRunAs,
		AllowedImagePrefixes: allowed,
		DockerOk:             err == nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
