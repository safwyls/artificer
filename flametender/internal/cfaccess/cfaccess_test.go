package cfaccess

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer = "https://emberhold.cloudflareaccess.com"
	testAUD    = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
)

type signer struct {
	kid string
	key *rsa.PrivateKey
}

func newSigner(t *testing.T, kid string) signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return signer{kid: kid, key: key}
}

func (s signer) jwk() map[string]string {
	return map[string]string{
		"kid": s.kid,
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
	}
}

// token mints an Access-shaped assertion. Defaults are valid; each test
// bends exactly one thing so a failure names its own cause.
func (s signer) token(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	base := jwt.MapClaims{
		"iss":   testIssuer,
		"aud":   []string{testAUD},
		"email": "Ember@example.com",
		"sub":   "0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9",
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
		"type":  "app",
	}
	for k, v := range claims {
		if v == nil {
			delete(base, k)
			continue
		}
		base[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, base)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// newVerifier wires a verifier to a fake certs endpoint publishing the
// given signers, and reports how many times it was fetched.
func newVerifier(t *testing.T, signers ...signer) (*Verifier, *atomic.Int32) {
	t.Helper()
	var fetches atomic.Int32
	current := signers
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		keys := make([]map[string]string, 0, len(current))
		for _, s := range current {
			keys = append(keys, s.jwk())
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(srv.Close)

	v, err := New("emberhold.cloudflareaccess.com", testAUD)
	if err != nil {
		t.Fatal(err)
	}
	// Point the key fetch at the fake; the issuer stays the real one so
	// the issuer check is still under test.
	v.certsURL = srv.URL
	return v, &fetches
}

func TestVerifyAcceptsATokenFromTheTeam(t *testing.T) {
	s := newSigner(t, "key-1")
	v, fetches := newVerifier(t, s)

	id, err := v.Verify(context.Background(), s.token(t, nil))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	// Lowercased on the way in: identity providers vary on case, and an
	// address that matches a user row only sometimes is worse than one
	// that never does.
	if id.Email != "ember@example.com" {
		t.Errorf("email = %q, want it lowercased", id.Email)
	}
	if id.Subject == "" {
		t.Error("subject was dropped")
	}
	if id.IsServiceToken() {
		t.Error("a token with an email is not a service token")
	}
	// The second verification uses the cached key.
	if _, err := v.Verify(context.Background(), s.token(t, nil)); err != nil {
		t.Fatal(err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("certs fetched %d times, want 1 — the key cache isn't holding", got)
	}
}

// The subtle one. A token minted for a *different Access application in
// the same team* carries a signature from these very keys and a valid
// issuer; only the audience says it wasn't meant for this console.
func TestVerifyRejectsAnotherApplicationsToken(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	_, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{
		"aud": []string{"9999999999999999999999999999999999999999999999999999999999999999"},
	}))
	if err == nil {
		t.Fatal("a token for another application was accepted")
	}
}

func TestVerifyRejectsAnotherTeamsIssuer(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{
		"iss": "https://someone-else.cloudflareaccess.com",
	})); err == nil {
		t.Fatal("a token from another team was accepted")
	}
}

func TestVerifyRejectsExpiredAndUnexpiring(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	// One second past expiry is expired. The clock-skew tolerance must not
	// leak into this direction, or every token quietly lives five minutes
	// longer than Cloudflare intended.
	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{
		"exp": time.Now().Add(-time.Second).Unix(),
	})); err == nil {
		t.Error("a just-expired token was accepted — skew tolerance is leaking into expiry")
	}
	// A token with no expiry would be a permanent credential in a header.
	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{"exp": nil})); err == nil {
		t.Error("a token with no expiry was accepted")
	}
}

// The other direction of the same tolerance: Access sets nbf == iat, so a
// slightly fast origin clock sees brand-new tokens as not-yet-valid.
// Rejecting those would look like "SSO randomly doesn't work".
func TestVerifyToleratesAFastOriginClock(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	soon := time.Now().Add(30 * time.Second).Unix()
	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{
		"iat": soon, "nbf": soon,
	})); err != nil {
		t.Errorf("a token minted seconds ahead of our clock was rejected: %v", err)
	}
}

// An origin should only ever be shown an application token; the team-wide
// session token is a different credential with a different scope.
func TestVerifyRejectsTheOrgSessionToken(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{"type": "org"})); err == nil {
		t.Fatal("the team-wide session token was accepted as an application token")
	}
}

// Algorithm confusion: an attacker who knows the public key tries to sign
// with it as an HMAC secret. Pinning RS256 is what refuses this before the
// key is ever looked up.
func TestVerifyRejectsASymmetricallySignedToken(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": testIssuer, "aud": []string{testAUD}, "email": "attacker@example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "key-1"
	signed, err := tok.SignedString(s.key.N.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), signed); err == nil {
		t.Fatal("an HS256 token was accepted — the algorithm is not pinned")
	}
}

func TestVerifyRejectsAForeignSignature(t *testing.T) {
	published, attacker := newSigner(t, "key-1"), newSigner(t, "key-1")
	v, _ := newVerifier(t, published)

	// Same key id, different private key: the signature must not verify.
	if _, err := v.Verify(context.Background(), attacker.token(t, nil)); err == nil {
		t.Fatal("a token signed by an unpublished key was accepted")
	}
}

// Key rotation: Cloudflare publishes a new key, tokens start arriving with
// a kid we've never seen, and one refetch picks it up without a restart.
func TestVerifyRefetchesOnAnUnknownKeyID(t *testing.T) {
	old, fresh := newSigner(t, "key-old"), newSigner(t, "key-new")
	v, fetches := newVerifier(t, old)

	if _, err := v.Verify(context.Background(), old.token(t, nil)); err != nil {
		t.Fatal(err)
	}
	// The team rotates: the endpoint now publishes both.
	v.certsURL = serveKeys(t, old, fresh)
	// Rate limiting would otherwise refuse the refetch we're testing.
	v.mu.Lock()
	v.fetchedAt = time.Now().Add(-2 * minRefetchInterval)
	v.mu.Unlock()

	if _, err := v.Verify(context.Background(), fresh.token(t, nil)); err != nil {
		t.Fatalf("a rotated key was not picked up: %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("original endpoint fetched %d times", got)
	}
}

// An unknown key id is exactly what a forged token produces, so it must
// not be a free way to make this console hammer Cloudflare.
func TestUnknownKeyIDRefetchIsRateLimited(t *testing.T) {
	s := newSigner(t, "key-1")
	v, fetches := newVerifier(t, s)

	forged := newSigner(t, "made-up-kid")
	for i := 0; i < 5; i++ {
		if _, err := v.Verify(context.Background(), forged.token(t, nil)); err == nil {
			t.Fatal("a token signed by an unknown key was accepted")
		}
	}
	if got := fetches.Load(); got > 1 {
		t.Errorf("certs fetched %d times for 5 forged tokens — refresh isn't rate limited", got)
	}
}

func TestVerifyRejectsATokenNamingNobody(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	if _, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{"email": nil})); err == nil {
		t.Fatal("a token with no identity was accepted")
	}
}

// Service tokens authenticate machines, not people: no email, a
// common_name instead. The verifier surfaces them honestly and leaves the
// policy call to the caller.
func TestServiceTokenIsIdentifiedAsOne(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	id, err := v.Verify(context.Background(), s.token(t, jwt.MapClaims{
		"email": nil, "common_name": "ci-runner.example",
	}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !id.IsServiceToken() || id.CommonName != "ci-runner.example" {
		t.Errorf("identity = %+v, want a service token", id)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	s := newSigner(t, "key-1")
	v, _ := newVerifier(t, s)

	for _, raw := range []string{"", "   ", "not.a.token", "a.b"} {
		if _, err := v.Verify(context.Background(), raw); err == nil {
			t.Errorf("%q was accepted", raw)
		}
	}
}

func TestTokenFromPrefersTheHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	if got := TokenFrom(r); got != "" {
		t.Errorf("a request with no assertion returned %q", got)
	}
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "from-cookie"})
	if got := TokenFrom(r); got != "from-cookie" {
		t.Errorf("cookie fallback = %q", got)
	}
	r.Header.Set(HeaderName, "from-header")
	if got := TokenFrom(r); got != "from-header" {
		t.Errorf("header should win: %q", got)
	}
}

// One malformed key must not take the whole published set down with it.
func TestParseJWKSSkipsUnusableKeys(t *testing.T) {
	good := newSigner(t, "good")
	body := fmt.Sprintf(`{"keys":[
	  {"kid":"","kty":"RSA","alg":"RS256","n":"AQAB","e":"AQAB"},
	  {"kid":"ec","kty":"EC","alg":"ES256","n":"AQAB","e":"AQAB"},
	  {"kid":"bad-n","kty":"RSA","alg":"RS256","n":"!!!not base64!!!","e":"AQAB"},
	  %s
	]}`, mustJSON(good.jwk()))

	keys, err := parseJWKS([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys["good"] == nil {
		t.Errorf("keys = %v, want only the usable one", keys)
	}
}

func TestNewRequiresBothHalves(t *testing.T) {
	if _, err := New("", testAUD); err == nil {
		t.Error("a verifier with no team domain was built")
	}
	// Without the audience every application in the team would be a way in.
	if _, err := New("emberhold.cloudflareaccess.com", ""); err == nil {
		t.Error("a verifier with no audience was built")
	}
}

func TestLogoutURLEndsTheAccessSession(t *testing.T) {
	v, _ := newVerifier(t, newSigner(t, "key-1"))
	if !strings.HasSuffix(v.LogoutURL(), "/cdn-cgi/access/logout") {
		t.Errorf("logout URL = %q", v.LogoutURL())
	}
}

func serveKeys(t *testing.T, signers ...signer) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys := make([]map[string]string, 0, len(signers))
		for _, s := range signers {
			keys = append(keys, s.jwk())
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
