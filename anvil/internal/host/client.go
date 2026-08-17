package host

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// Registered clients: one per game console, each with its own token.
//
// # Why per-console rather than one shared token
//
// A shared token cannot express ownership. Every console would be able to
// destroy or rebuild every other console's servers, and the owner label
// would be a comment rather than a rule — which is exactly what it was in
// the first cut of this service.
//
// With a token per console the label becomes enforceable: the token says
// who you are, this service decides what that entitles you to, and a leaked
// wildskeeper token cannot take down a Palworld server. The contracts stay
// deliberately similar — same verbs, same spec, same shapes — but they are
// separate agreements with separate credentials, and neither console can
// speak for the other.
//
// Each client also brings its own data root and its own image allowlist,
// which is the same idea applied twice: the console that only ever deploys
// wkagent images should only be *able* to deploy wkagent images, and a
// console that places servers under one directory has no business writing
// to another's.
//
// # What is deliberately still shared
//
// Ports and container names. A console has to be able to see that 8211 is
// taken and by what, or the collision this service exists to prevent comes
// straight back — see `/v1/ports` and the foreign rows in `/v1/containers`.
// Those carry no slug, image, data directory or environment: enough to
// avoid a collision and explain one, and nothing more.

// ClientConfig is one console's registration.
type ClientConfig struct {
	// ID is the console's name, and becomes the owner label on everything
	// it places: "wildskeeper", "palcon".
	ID string `json:"id"`
	// Token is that console's bearer token. At least minTokenLen.
	Token string `json:"token"`
	// DataRoot is where this console's server data directories live. The
	// consoles keep theirs in different places, so this is per client
	// rather than global.
	DataRoot string `json:"dataRoot"`
	// ImagePrefixes bounds what this console may deploy. Empty falls back
	// to the service-wide allowlist.
	ImagePrefixes []string `json:"imagePrefixes,omitempty"`
	// EnvPrefix is this console's environment namespace ("WKAGENT_",
	// "PALAGENT_"). Adopt returns only variables under it; a client
	// registered without one gets no environment back at all.
	EnvPrefix string `json:"envPrefix,omitempty"`
}

var clientIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func (c ClientConfig) validate() error {
	if !clientIDPattern.MatchString(c.ID) {
		return fmt.Errorf("client id %q must be 1-32 lowercase letters, digits or dashes", c.ID)
	}
	if len(c.Token) < minTokenLen {
		return fmt.Errorf("client %q: token must be at least %d characters", c.ID, minTokenLen)
	}
	if c.DataRoot == "" {
		return fmt.Errorf("client %q: data root is required", c.ID)
	}
	return nil
}

// client is a registered console at runtime. The token is kept hashed so a
// heap dump or a stray log line can't spill it.
type client struct {
	ID            string
	tokenHash     [32]byte
	DataRoot      string
	ImagePrefixes []string
	EnvPrefix     string
}

// LoadClients reads registrations from JSON — either inline or from a file.
// A file is the better habit for secrets, so it wins when both are set.
func LoadClients(inline, path string) ([]ClientConfig, error) {
	data := []byte(strings.TrimSpace(inline))
	if path != "" {
		read, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading clients file: %w", err)
		}
		data = read
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []ClientConfig
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing clients: %w", err)
	}
	return out, nil
}

// resolve identifies the caller from its bearer token.
//
// The comparison is constant-time against every registered client, and it
// does not stop early on a match: a timing difference between "first client
// matched" and "last client matched" would leak which console a token
// belongs to, which is a small thing but a free one to avoid.
func (s *Service) resolve(r *http.Request) *client {
	presented := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
	var found *client
	for i := range s.clients {
		if subtle.ConstantTimeCompare(presented[:], s.clients[i].tokenHash[:]) == 1 {
			found = &s.clients[i]
		}
	}
	return found
}

// callerKey is the context key holding the resolved client.
type callerKey struct{}

// caller returns the console that made this request. Never nil inside a
// handler: the middleware rejects anything it cannot identify.
func caller(r *http.Request) *client {
	c, _ := r.Context().Value(callerKey{}).(*client)
	return c
}

// ownerOf reports which console owns a container, from its labels.
//
// The legacy per-console labels carry the answer already — a container
// tagged wildskeeper.provisioned was made by wildskeeper — so servers that
// predate this service arrive with their ownership intact rather than
// needing to be claimed or relabelled.
func ownerOf(labels map[string]string) (owner, slug string, ok bool) {
	if labels[LabelManaged] == "true" {
		return labels[LabelOwner], labels[LabelSlug], true
	}
	// Containers this service made under its former name (ilmari) carry the
	// old label namespace. Recognising them keeps a live host's managed
	// servers from being orphaned when the service is renamed — the same
	// courtesy the per-console legacy labels below already get. New
	// containers get anvil.* above; these are only ever read, never written.
	if labels["ilmari.managed"] == "true" {
		return labels["ilmari.owner"], labels["ilmari.slug"], true
	}
	for i, key := range legacyManagedLabels {
		if labels[key] == "true" {
			return legacyOwners[i], labels[legacySlugLabels[i]], true
		}
	}
	return "", "", false
}

// errForeign is returned when a console reaches for a container that is not
// its own.
var errForeign = errors.New("that container belongs to a different console")
