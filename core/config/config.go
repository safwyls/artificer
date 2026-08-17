package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all runtime configuration, sourced entirely from environment
// variables so the container can be configured without a config file.
type Config struct {
	// HTTPAddr is the address the HTTP server listens on, e.g. ":8080".
	HTTPAddr string

	// DataDir is where the sqlite database lives. Mount this as a volume.
	DataDir string

	// JWTSecret signs session cookies. Must stay stable across restarts or
	// existing sessions are invalidated.
	JWTSecret []byte

	// EncryptionKey encrypts stored RCON/REST passwords at rest. Must be
	// exactly 32 bytes. Losing this key makes stored server credentials
	// unrecoverable, so back it up alongside the database.
	EncryptionKey []byte

	// Bootstrap admin, only used the first time the app starts (when the
	// users table is empty).
	AdminUsername string
	AdminPassword string

	// DockerHost points at a scoped docker socket proxy used to start and
	// stop game server containers. Empty disables power control entirely —
	// Flametender should never require access to a docker socket to run.
	DockerHost string

	// AnvilURL/Token point at the shared Anvil host service
	// (github.com/safwyls/anvil) — the only component with Docker rights,
	// and this console's only way to place containers. Empty means the
	// Raise-a-server wizard is absent and servers are registered by hand.
	AnvilURL   string
	AnvilToken string

	// AnthropicAPIKey / GeminiAPIKey enable the advisor chat — set one or
	// the other. Both empty leaves the feature to a key saved through the
	// admin UI; absent everywhere means the UI never offers it.
	AnthropicAPIKey string
	GeminiAPIKey    string

	// CookieSecure marks the session cookie Secure for deployments behind
	// TLS. Off by default so plain-HTTP LAN setups keep working.
	CookieSecure bool

	// Cloudflare Access single sign-on. Setting both TeamDomain and AUD
	// turns it on; unset means the console only knows password login.
	//
	// AccessTeamDomain is the team's Access hostname
	// ("yourteam.cloudflareaccess.com"), which is both the token issuer
	// and where its signing keys are published. AccessAUD is the
	// Application Audience tag of the *specific* Access application in
	// front of this console — a token minted for another app in the same
	// team carries a valid signature, so the audience check is what stops
	// it being accepted here.
	AccessTeamDomain string
	AccessAUD        string
	// AccessAdminEmails are addresses that hold the admin role whenever
	// they sign in through Access, re-applied on every login. It exists
	// so an operator cannot lock themselves out of their own console: the
	// alternative is a first SSO user with no rights and nobody able to
	// grant them.
	AccessAdminEmails []string
}

func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "flametender.db")
}

// Load reads configuration from the environment. Required variables are
// JWT_SECRET and ENCRYPTION_KEY; everything else has a sane default for
// local development.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		DataDir:       getEnv("DATA_DIR", "./data"),
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		DockerHost:    os.Getenv("DOCKER_HOST"),
		// One-click provisioning rides the shared Anvil host service;
		// when unset, the new-server wizard is simply absent.
		AnvilURL:   os.Getenv("ANVIL_URL"),
		AnvilToken: os.Getenv("ANVIL_TOKEN"),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		// Cloudflare Access SSO; both required to enable it.
		AccessTeamDomain:  normalizeTeamDomain(os.Getenv("CF_ACCESS_TEAM_DOMAIN")),
		AccessAUD:         strings.TrimSpace(os.Getenv("CF_ACCESS_AUD")),
		AccessAdminEmails: splitEmails(os.Getenv("CF_ACCESS_ADMIN_EMAILS")),
	}

	cfg.CookieSecure = os.Getenv("COOKIE_SECURE") == "true" || os.Getenv("COOKIE_SECURE") == "1"

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	// A short secret makes session forgery brute-forceable; .env.example
	// already tells people to generate 32+ chars, so enforce it.
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters, got %d (generate one with `openssl rand -hex 32`)", len(jwtSecret))
	}
	cfg.JWTSecret = []byte(jwtSecret)

	encKey := os.Getenv("ENCRYPTION_KEY")
	if len(encKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required and must be exactly 32 bytes, got %d", len(encKey))
	}
	cfg.EncryptionKey = []byte(encKey)

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// AccessEnabled reports whether Cloudflare Access SSO is configured. Both
// halves are required: without the audience tag the console would accept
// any token the team ever minted, for any application.
func (c *Config) AccessEnabled() bool {
	return c.AccessTeamDomain != "" && c.AccessAUD != ""
}

// normalizeTeamDomain accepts what an operator is likely to paste — a bare
// team name, the full Access hostname, or either with a scheme or trailing
// slash — and yields the hostname the issuer and certs URL are built from.
func normalizeTeamDomain(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
	v = strings.TrimSuffix(v, "/")
	if !strings.Contains(v, ".") {
		v += ".cloudflareaccess.com"
	}
	return v
}

// splitEmails parses a comma-separated list, lowercasing as it goes:
// identity providers vary on case and an address that fails to match here
// silently costs someone their admin role.
func splitEmails(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if e := strings.ToLower(strings.TrimSpace(part)); e != "" {
			out = append(out, e)
		}
	}
	return out
}

// retiredEnv names settings this console no longer reads, each with what
// replaced it. Renames and removals both land here.
var retiredEnv = []struct{ old, replacement, why string }{
	{"ILMARI_URL", "ANVIL_URL", "the host service is now called anvil"},
	{"ILMARI_TOKEN", "ANVIL_TOKEN", "the host service is now called anvil"},
	{"PROVISIONER_URL", "ANVIL_URL", "provisioner-mode agents are retired; provisioning goes through anvil"},
	{"PROVISIONER_TOKEN", "ANVIL_TOKEN", "provisioner-mode agents are retired; provisioning goes through anvil"},
}

// RetiredSettings reports environment variables that are set but no longer
// read, and whose replacement is missing — so a console can say so at boot.
//
// This exists because the failure mode is silence. A deployment upgraded
// with only the old names loses provisioning entirely: the wizard is
// simply absent, which looks like a bug in the console rather than an
// unset variable. Naming it costs one log line. Variables whose
// replacement is already set are not reported: that is a harmless
// leftover, not a problem to fix.
func RetiredSettings() []string {
	var out []string
	for _, r := range retiredEnv {
		if os.Getenv(r.old) != "" && os.Getenv(r.replacement) == "" {
			out = append(out, r.old+" is no longer read — set "+r.replacement+" instead ("+r.why+")")
		}
	}
	return out
}
