// Package cfaccess verifies Cloudflare Access application tokens, so a
// console behind a Cloudflare Tunnel can trust the identity Access has
// already authenticated.
//
// # How the identity arrives
//
// When a request passes an Access policy, Cloudflare adds a signed JWT:
// the `Cf-Access-Jwt-Assertion` header, and for browser navigations a
// `CF_Authorization` cookie carrying the same token. The token names the
// person (`email`) or the machine (`common_name`, for service tokens),
// and is signed with RS256 by a key the team publishes at
// `https://<team>.cloudflareaccess.com/cdn-cgi/access/certs`.
//
// # Why the signature is not optional
//
// A header is only a header. Anything that can reach this origin
// directly — another container on the host, someone on the LAN, a
// misrouted port forward — can send `Cf-Access-Jwt-Assertion: whatever`.
// Trusting it unverified would turn "SSO" into "type any email you
// like". So the token is verified cryptographically on every use, and
// two checks beyond the signature carry real weight:
//
//   - **Issuer**, so a token from some other team is not accepted.
//   - **Audience**, so a token minted for a *different application in
//     the same team* is not accepted. This is the subtle one: those
//     tokens have a perfectly valid signature from the same keys. The
//     AUD tag of this console's Access application is what separates
//     "authenticated by my team" from "authorized for this app".
//
// What this package cannot do is stop someone who reaches the origin
// directly from using the console's own password login. That is why
// password login remains — as break-glass — rather than being replaced;
// see docs/cloudflare-access.md.
package cfaccess

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// HeaderName is where Access puts the token on every proxied request.
const HeaderName = "Cf-Access-Jwt-Assertion"

// CookieName is the same token as a cookie, which is what a browser
// navigation carries. Read as a fallback: the header is present on
// requests Access proxies, but a cached SPA calling the API can arrive
// with only the cookie.
const CookieName = "CF_Authorization"

const (
	// maxCertsBytes bounds the JWKS response; a real one is a few KB.
	maxCertsBytes = 1 << 20
	// minRefetchInterval rate-limits key refreshes. An unknown `kid` is
	// the signal to refetch, and an unknown kid is also exactly what an
	// attacker can produce at will — without this, forged tokens would be
	// a free way to make this console hammer Cloudflare.
	minRefetchInterval = time.Minute
	// maxKeyAge refetches on a slow schedule even when every kid resolves,
	// so a key retired between rotations doesn't linger indefinitely.
	maxKeyAge = 12 * time.Hour
)

// Identity is who Access says is calling.
type Identity struct {
	// Email is the person's address, absent for service tokens.
	Email string
	// Subject is Cloudflare's id for the user within the account. It is
	// deliberately *not* used as this console's account key: Cloudflare
	// documents it as changing if a user is removed and re-added, so an
	// account keyed on it would silently orphan itself. The email is the
	// key; this is for logs.
	Subject string
	// CommonName is the service token's client id, set when the caller
	// authenticated as a *machine* rather than a person. Callers decide
	// what to do with that; this console refuses them, having no
	// machine-caller story.
	CommonName string
	// Type is the token's own `type` claim: "app" for an application
	// token (the only kind an origin should ever see) or "org" for the
	// team-wide session token.
	Type string
	// ExpiresAt is the token's own expiry, for logging and tests.
	ExpiresAt time.Time
}

// IsServiceToken reports a machine caller.
//
// The test is the presence of `common_name` rather than the absence of an
// email, on Cloudflare's own guidance: it is the claim that positively
// identifies a service token, and it names *which* token. Testing for a
// missing email instead would conflate a machine with a person whose
// identity provider simply released no address — and in an authentication
// path, the conservative reading is the right one.
func (i *Identity) IsServiceToken() bool { return i.CommonName != "" }

// Verifier validates tokens for one Access application.
type Verifier struct {
	issuer   string
	certsURL string
	audience string
	http     *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// New builds a verifier for a team domain ("yourteam.cloudflareaccess.com")
// and the Access application's audience tag. Both are required: without
// the audience, every application in the team would be a way in.
func New(teamDomain, audience string) (*Verifier, error) {
	teamDomain = strings.TrimSuffix(strings.TrimSpace(teamDomain), "/")
	audience = strings.TrimSpace(audience)
	if teamDomain == "" {
		return nil, errors.New("cloudflare access: team domain is required")
	}
	if audience == "" {
		return nil, errors.New("cloudflare access: application audience (AUD) is required")
	}
	return &Verifier{
		issuer:   "https://" + teamDomain,
		certsURL: "https://" + teamDomain + "/cdn-cgi/access/certs",
		audience: audience,
		http:     &http.Client{Timeout: 10 * time.Second},
		keys:     map[string]*rsa.PublicKey{},
	}, nil
}

// Issuer is the team's Access URL, which tokens must name as their issuer.
func (v *Verifier) Issuer() string { return v.issuer }

// LogoutURL ends the Access session itself. Clearing this console's own
// cookie is not a sign-out on a shared machine: Access would silently
// hand the next person the same identity.
//
// Two properties of it are worth stating where callers will see them,
// because neither is obvious and both are Cloudflare's design rather than
// ours: it signs the person out of *every* Access application, there
// being no per-application logout; and already-issued tokens keep
// verifying for another 20–30 seconds afterwards.
func (v *Verifier) LogoutURL() string { return v.issuer + "/cdn-cgi/access/logout" }

// TokenFrom pulls the assertion off a request, header first.
func TokenFrom(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get(HeaderName)); t != "" {
		return t
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

type accessClaims struct {
	Email      string `json:"email"`
	CommonName string `json:"common_name"`
	// Type separates an application token from the team-wide session
	// token ("org") that Access sets on its own domain.
	Type string `json:"type"`
	jwt.RegisteredClaims
}

// clockSkew is the tolerance for a *freshly minted* token. Access sets
// `nbf` equal to `iat`, so an origin clock a few seconds fast would
// reject brand-new tokens outright — a failure that would present as
// "SSO randomly doesn't work". Cloudflare's own Go example (via go-oidc)
// allows five minutes here.
//
// It is deliberately one-directional: see the expiry check in Verify.
// Being generous about a token that is not valid *yet* costs nothing,
// while being generous about one that has already expired hands out extra
// life to a credential its issuer has finished with.
const clockSkew = 5 * time.Minute

// ErrNoToken means the request carried no assertion at all — normally
// "this request didn't come through Access", not a failure worth alarming
// about.
var ErrNoToken = errors.New("no Cloudflare Access assertion on the request")

// Verify checks a token and returns who it names.
//
// Every check is mandatory: RS256 only (never let a token choose its own
// algorithm), a key from this team's published set, the issuer, the
// audience of this specific application, and expiry.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Identity, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, ErrNoToken
	}
	claims := &accessClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, v.keyFunc(ctx),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(clockSkew),
	)
	if err != nil {
		return nil, fmt.Errorf("cloudflare access token rejected: %w", err)
	}
	// The leeway above applies to every time claim, expiry included, so
	// expiry is re-checked here without it. golang-jwt has no per-claim
	// leeway, and "tolerate a fast clock" must not quietly become
	// "tolerate a stale token".
	if claims.ExpiresAt == nil || !claims.ExpiresAt.After(time.Now()) {
		return nil, errors.New("cloudflare access token has expired")
	}
	// Cheap defence in depth: an origin should only ever be shown an
	// application token. The team-wide session token ("org") is a
	// different credential with a different scope, and the audience check
	// above should already exclude it — this makes that explicit rather
	// than incidental.
	if claims.Type != "" && claims.Type != "app" {
		return nil, fmt.Errorf("cloudflare access token is of type %q, not an application token", claims.Type)
	}
	id := &Identity{
		Email:      strings.ToLower(strings.TrimSpace(claims.Email)),
		Subject:    claims.Subject,
		CommonName: strings.TrimSpace(claims.CommonName),
		Type:       claims.Type,
	}
	if claims.ExpiresAt != nil {
		id.ExpiresAt = claims.ExpiresAt.Time
	}
	if id.Email == "" && id.CommonName == "" {
		// A validly signed token that names nobody. Nothing sensible can
		// be done with it, and inventing an identity is the one thing an
		// authentication path must never do.
		return nil, errors.New("cloudflare access token carries no email or service-token name")
	}
	return id, nil
}

// keyFunc resolves the token's `kid` against the team's published keys,
// refetching once when the key is unknown — that is how a rotation is
// picked up without a restart.
func (v *Verifier) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no key id")
		}
		if key := v.cachedKey(kid); key != nil {
			return key, nil
		}
		if err := v.refresh(ctx); err != nil {
			return nil, err
		}
		if key := v.cachedKey(kid); key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("no Cloudflare Access signing key %q", kid)
	}
}

func (v *Verifier) cachedKey(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if time.Since(v.fetchedAt) > maxKeyAge {
		return nil
	}
	return v.keys[kid]
}

// refresh reloads the team's signing keys, rate-limited so an attacker
// cannot turn forged key ids into traffic against Cloudflare.
func (v *Verifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	if time.Since(v.fetchedAt) < minRefetchInterval {
		v.mu.Unlock()
		return errors.New("Cloudflare Access signing keys were refreshed moments ago; the key id is unknown")
	}
	// Stamp before releasing: two concurrent unknown-kid requests should
	// produce one fetch, not two.
	v.fetchedAt = time.Now()
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetching Cloudflare Access keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching Cloudflare Access keys: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCertsBytes))
	if err != nil {
		return fmt.Errorf("reading Cloudflare Access keys: %w", err)
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("Cloudflare Access published no usable signing keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

// jwks is the certs endpoint's payload. Only the standard `keys` array is
// read; the endpoint also returns PEM certificates for callers that want
// them, which this package does not.
type jwks struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwks
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decoding Cloudflare Access keys: %w", err)
	}
	out := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		// Skip rather than fail: an unusable key alongside good ones must
		// not take the whole set down.
		if k.Kid == "" || (k.Kty != "" && k.Kty != "RSA") || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		n, err := b64(k.N)
		if err != nil {
			continue
		}
		e, err := b64(k.E)
		if err != nil || len(e) == 0 {
			continue
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() > 1<<31-1 || exponent.Int64() < 3 {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(n),
			E: int(exponent.Int64()),
		}
	}
	return out, nil
}

// b64 decodes base64url with or without padding — JWKs are specified
// unpadded, but accepting both costs nothing and survives a producer that
// pads.
func b64(s string) ([]byte, error) {
	if v, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return v, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
