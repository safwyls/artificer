// Package anvil is the client for the Anvil host-provisioning service
// (github.com/safwyls/anvil) — the shared, game-agnostic replacement for
// the per-console provisioner-mode agents.
//
// The division of knowledge is the point of the wire shapes here: Anvil
// knows containers, images, ports and data directories, and this console
// knows what a Dragonwilds server is made of. So everything in this package
// is deliberately dumb — specs in, results out — and the translation from
// "a server named Ashenfall on port 7777" into env vars and port maps lives
// with the caller (internal/api's provisioner adapter), not here and never
// in Anvil.
package anvilclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The error taxonomy. Anvil answers a destroy with 404 when the container
// is already gone and 403 when it exists but belongs to another console,
// and those two want opposite handling: the first is the end state the
// caller asked for and the server row should go, the second must leave
// everything exactly where it is. Folding both into one flat error — which
// is what this package did before — makes a console unable to tell a
// finished job from a forbidden one, and the "already gone, delete the
// row" path in api/servers.go silently stopped working when provisioning
// moved to Anvil.
//
// These mirror agentctl's sentinels rather than reusing them: this package
// speaks to Anvil, not to a sidecar agent, and the adapter in core/api
// translates. That keeps the two protocols' error vocabularies from
// drifting into each other.
var (
	// ErrNotFound is Anvil's 404 — no container by that name.
	ErrNotFound = errors.New("not found on the host")
	// ErrRejected is a refusal Anvil is sure about: a bad request, a
	// rejected token, or a container owned by a different console.
	// Retrying it unchanged cannot help.
	ErrRejected = errors.New("anvil refused the request")
	// ErrConflict is Anvil's 409: the host already holds the name or the
	// ports. Nothing was created — see Reason for which.
	ErrConflict = errors.New("conflict on the host")
)

// APIVersion is the Anvil protocol this client speaks. Anvil reports its
// own in /v1/health, and a mismatch is worth saying out loud at the point
// of contact rather than discovering later as a shape that won't parse.
const APIVersion = 1

// ErrAPIVersion reports an Anvil speaking a protocol this client does not.
var ErrAPIVersion = errors.New("anvil speaks a different protocol version")

// Conflict reasons, mirroring host.Reason* on the service. Absent from an
// older Anvil, in which case the reason is "".
const (
	ReasonNameTaken  = "name-taken"
	ReasonPortsInUse = "ports-in-use"
)

// ConflictError carries which kind of 409 this was, so a caller routes on
// a field instead of on the wording of a sentence.
type ConflictError struct {
	Reason string
	Msg    string
}

func (e *ConflictError) Error() string { return e.Msg }
func (e *ConflictError) Unwrap() error { return ErrConflict }

// ConflictReason reports the reason on a conflict error, or "" if err is
// not one (or came from an Anvil too old to say).
func ConflictReason(err error) string {
	var ce *ConflictError
	if errors.As(err, &ce) {
		return ce.Reason
	}
	return ""
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(baseURL, token string) (*Client, error) {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("anvil url is empty")
	}
	if token == "" {
		return nil, errors.New("anvil token is empty")
	}
	// No client-wide timeout: each call sets its own, because a single
	// deadline can't cover both a fast health read and a provision that
	// legitimately spends minutes pulling an image.
	return &Client{base: baseURL, token: token, http: &http.Client{}}, nil
}

func (c *Client) BaseURL() string { return c.base }

func (c *Client) do(ctx context.Context, method, path string, in, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("anvil unreachable: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(data, &e)
		msg := e.Error
		if msg == "" {
			msg = resp.Status
		}
		switch resp.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, msg)
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: anvil rejected the token — re-check it on both sides", ErrRejected)
		case http.StatusForbidden, http.StatusBadRequest:
			return fmt.Errorf("%w: %s", ErrRejected, msg)
		case http.StatusConflict:
			return &ConflictError{Reason: e.Reason, Msg: ErrConflict.Error() + ": " + msg}
		}
		// 5xx and anything unexpected stay untyped: they are neither a
		// refusal nor an already-done, and a caller should treat them as
		// "unknown, try again later" rather than as either.
		return fmt.Errorf("anvil: %s", msg)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Health mirrors Anvil's /v1/health — already scoped to this client's
// registration (its own data root, its own allowlist).
type Health struct {
	Service              string   `json:"service"`
	Version              string   `json:"version"`
	APIVersion           int      `json:"apiVersion"`
	Client               string   `json:"client"`
	DataRoot             string   `json:"dataRoot"`
	PublicHost           string   `json:"publicHost"`
	RunAs                string   `json:"runAs"`
	AllowedImagePrefixes []string `json:"allowedImagePrefixes"`
	DockerOk             bool     `json:"dockerOk"`
}

func (c *Client) Health(ctx context.Context) (*Health, error) {
	var h Health
	if err := c.do(ctx, http.MethodGet, "/v1/health", nil, &h, 10*time.Second); err != nil {
		return nil, err
	}
	// A newer Anvil may add fields this client ignores, which is fine; a
	// different major is not, and provisioning against one would fail in
	// some more confusing way further in. Zero means an Anvil old enough
	// not to report it at all — from before the field existed — and is
	// accepted rather than guessed about.
	if h.APIVersion != 0 && h.APIVersion != APIVersion {
		return nil, fmt.Errorf("%w: anvil speaks v%d, this console speaks v%d — upgrade whichever is older",
			ErrAPIVersion, h.APIVersion, APIVersion)
	}
	return &h, nil
}

// PortMap publishes one container port on the host.
type PortMap struct {
	Host      int    `json:"host"`
	Container int    `json:"container"`
	Proto     string `json:"proto,omitempty"`
}

// Spec is Anvil's provisioning contract: everything the game needs, as
// data. See the package comment for who fills it in.
type Spec struct {
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Image     string            `json:"image"`
	User      string            `json:"user,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Ports     []PortMap         `json:"ports,omitempty"`
	DataMount string            `json:"dataMount,omitempty"`
}

type PlaceResult struct {
	Container string `json:"container"`
	DataDir   string `json:"dataDir"`
	Image     string `json:"image"`
}

// Provision places one container. The generous timeout covers the image
// pull a first placement performs.
func (c *Client) Provision(ctx context.Context, spec Spec) (*PlaceResult, error) {
	var res PlaceResult
	if err := c.do(ctx, http.MethodPost, "/v1/provision", spec, &res, 10*time.Minute); err != nil {
		return nil, err
	}
	return &res, nil
}

type RecreateResult struct {
	Container string `json:"container"`
	Image     string `json:"image"`
	Previous  string `json:"previousImage"`
}

// Recreate rebuilds a container on a different image, keeping everything
// else. Long timeout for the same reason as Provision: the Wine image is
// over a gigabyte and a slow pull is not a failure.
func (c *Client) Recreate(ctx context.Context, container, image string) (*RecreateResult, error) {
	var res RecreateResult
	body := map[string]string{"container": container, "image": image}
	if err := c.do(ctx, http.MethodPost, "/v1/provision/recreate", body, &res, 15*time.Minute); err != nil {
		return nil, err
	}
	return &res, nil
}

type DestroyResult struct {
	Container string `json:"container"`
	DataDir   string `json:"dataDir"`
}

// Destroy removes a container, keeping its data directory. The budget
// covers a stop that legitimately uses its full grace period.
func (c *Client) Destroy(ctx context.Context, container string) (*DestroyResult, error) {
	var res DestroyResult
	body := map[string]string{"container": container}
	if err := c.do(ctx, http.MethodPost, "/v1/provision/destroy", body, &res, 3*time.Minute); err != nil {
		return nil, err
	}
	return &res, nil
}

// Discovered is one adoption candidate: a container this console owns, or
// an unlabelled one under its image allowlist (a paste-flow deploy).
type Discovered struct {
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	Running bool      `json:"running"`
	Managed bool      `json:"managed"`
	Ports   []PortMap `json:"ports"`
}

func (c *Client) Discover(ctx context.Context) ([]Discovered, error) {
	var res struct {
		Servers []Discovered `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/discover", nil, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return res.Servers, nil
}

// Adopted is a recovered registration: the container plus its environment,
// filtered by Anvil to this console's registered namespace (FLAMEAGENT_*).
type Adopted struct {
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Running bool              `json:"running"`
	Ports   []PortMap         `json:"ports"`
	Env     map[string]string `json:"env"`
}

func (c *Client) Adopt(ctx context.Context, container string) (*Adopted, error) {
	var res Adopted
	body := map[string]string{"container": container}
	if err := c.do(ctx, http.MethodPost, "/v1/adopt", body, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return &res, nil
}

// ManagedContainer is one row of Anvil's fleet view: every container on the
// host, this console's or not.
//
// The foreign rows are the point. A console can only see its own servers,
// which is exactly the blindness that made two consoles collide on a host
// port — one proposed 8211, the other already had it, the create succeeded
// and the start failed. Anvil can see all of them, so it says so: name,
// image, ports, and for the caller's own rows a slug and data directory.
// Nothing else — a container's environment carries tokens and passwords,
// and a fleet view is not worth leaking them for.
type ManagedContainer struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Running bool   `json:"running"`
	// State is docker's lifecycle word (created, running, paused, exited);
	// Status its human sentence ("Up 3 hours", "Exited (137) 2 days ago"),
	// which is the only place the exit code and age travel. Both empty when
	// the Anvil predates them — Running is the fallback either way.
	State   string    `json:"state,omitempty"`
	Status  string    `json:"status,omitempty"`
	Created int64     `json:"created,omitempty"` // unix seconds
	Managed bool      `json:"managed"`
	Mine    bool      `json:"mine"`
	Slug    string    `json:"slug,omitempty"`
	Owner   string    `json:"owner,omitempty"`
	Ports   []PortMap `json:"ports,omitempty"`
	DataDir string    `json:"dataDir,omitempty"`
}

func (c *Client) Containers(ctx context.Context) ([]ManagedContainer, error) {
	var res struct {
		Containers []ManagedContainer `json:"containers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/containers", nil, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return res.Containers, nil
}

// HostImage is one image on the host's disk: its names, its size, and every
// container created from it. Shared visibility like ports — disk is spent
// host-wide, and a dangling image (no tags, no containers) is pure cost no
// container row can explain.
type HostImage struct {
	ID         string   `json:"id"`
	Tags       []string `json:"tags"`
	Size       int64    `json:"size"`    // bytes
	Created    int64    `json:"created"` // unix seconds
	Containers []string `json:"containers"`
}

// Images lists every image on the host, biggest first. An Anvil from before
// the endpoint existed answers 404, which arrives as ErrNotFound — callers
// should treat that as "this Anvil doesn't report images", not as an empty
// host.
func (c *Client) Images(ctx context.Context) ([]HostImage, error) {
	var res struct {
		Images []HostImage `json:"images"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/images", nil, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return res.Images, nil
}

// TakenPort is one published host port and what holds it.
type TakenPort struct {
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
	Container string `json:"container"`
}

// Ports reports every published host port on the machine — including ones
// held by other consoles and by containers nothing here manages. A port
// proposal built only from what this console tracks is a proposal that can
// collide, so this is what the wizard should count as taken.
func (c *Client) Ports(ctx context.Context) ([]TakenPort, error) {
	var res struct {
		Ports []TakenPort `json:"ports"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/ports", nil, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return res.Ports, nil
}
