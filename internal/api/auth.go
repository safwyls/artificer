package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/safwyls/flametender/internal/cfaccess"
	"github.com/safwyls/flametender/internal/store"
)

const sessionCookieName = "flametender_session"
const sessionDuration = 7 * 24 * time.Hour

type sessionClaims struct {
	// UserID, not the username: the request path reloads the user each
	// time so permission and password changes apply immediately, and a
	// rename doesn't invalidate a live session.
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	// SSO records that this session began at Cloudflare Access rather
	// than at the password form, so signing out can end that session too
	// — clearing only this console's cookie would leave the next person
	// at the same browser holding the previous one's identity.
	SSO bool `json:"sso,omitempty"`
	jwt.RegisteredClaims
}

func (s *Server) signSession(user *store.User, sso bool) (string, error) {
	claims := sessionClaims{
		UserID:   user.ID,
		Username: user.Username,
		SSO:      sso,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// setSessionCookie is the one place a session is handed out, so the two
// login paths cannot drift on cookie flags.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionDuration),
	})
}

func (s *Server) parseSession(tokenStr string) (*sessionClaims, error) {
	claims := &sessionClaims{}
	// Pin the algorithm we sign with; never let the token pick its own.
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ipKey := "ip:" + clientIP(r.RemoteAddr)
	userKey := "user:" + req.Username
	if !s.loginLimiter.allow(ipKey) || !s.loginLimiter.allow(userKey) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts — try again in a minute")
		return
	}

	user, err := s.store.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	// Deliberate: the distinct 403 (and the bcrypt-only-when-user-exists
	// timing) confirms the username exists. On a LAN admin tool that's an
	// acceptable trade — telling a locked-out player their account is
	// disabled rather than "wrong password" saves an admin round-trip.
	// Revisit if an instance is ever internet-facing.
	if user.Disabled {
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}
	s.loginLimiter.reset(ipKey)
	s.loginLimiter.reset(userKey)

	token, err := s.signSession(user, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

// ssoPasswordSentinel is stored as the password hash of an account
// created by Cloudflare Access. It is not a hash: bcrypt refuses to parse
// it, so every password comparison against it fails closed. That is the
// point — an SSO account has no password, and giving it an empty or
// guessable one would quietly open the break-glass door to everybody.
const ssoPasswordSentinel = "!sso-only-no-password"

// handleCloudflareLogin turns a verified Access assertion into a console
// session, creating the account the first time someone signs in.
//
// The assertion on the request *is* the credential, which is why this
// route sits outside requireAuth — and why nothing is read out of the
// token before cfaccess has checked its signature, issuer, audience and
// expiry. Anyone able to reach this origin directly can post whatever
// headers they like; only the cryptography makes that harmless.
func (s *Server) handleCloudflareLogin(w http.ResponseWriter, r *http.Request) {
	if s.Access == nil {
		writeError(w, http.StatusNotFound, "Cloudflare Access sign-in is not configured")
		return
	}
	identity, err := s.Access.Verify(r.Context(), cfaccess.TokenFrom(r))
	if errors.Is(err, cfaccess.ErrNoToken) {
		// The ordinary case for a request that didn't come through the
		// tunnel — the frontend tries this route before showing the
		// password form, and a LAN visitor simply falls through to it.
		writeError(w, http.StatusUnauthorized, "this request did not come through Cloudflare Access")
		return
	}
	if err != nil {
		s.logger.Warn("cloudflare access: assertion rejected", "error", err, "ip", clientIP(r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "Cloudflare Access assertion could not be verified")
		return
	}
	if identity.IsServiceToken() {
		// A machine got through the Access policy. There is no console
		// user it could be, and inventing one would be a back door with
		// good manners.
		writeError(w, http.StatusForbidden,
			"service tokens cannot sign in to Flametender — it has no machine accounts")
		return
	}

	user, err := s.store.GetUserByUsername(r.Context(), identity.Email)
	created := false
	if err != nil {
		// First sign-in: make the account. Deliberately with no
		// permissions — Access says who you are, never what you may do
		// here, and a console that granted power to everyone your
		// identity provider knows would be a surprise of the worst kind.
		role := ""
		if s.isAccessAdmin(identity.Email) {
			role = store.RoleAdmin
		}
		id, cerr := s.store.CreateUser(r.Context(), identity.Email, ssoPasswordSentinel, role, nil)
		if cerr != nil {
			s.logger.Error("cloudflare access: creating user", "email", identity.Email, "error", cerr)
			writeError(w, http.StatusInternalServerError, "could not create an account for this identity")
			return
		}
		user, err = s.store.GetUser(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load the new account")
			return
		}
		created = true
		s.logger.Info("cloudflare access: account created", "user", user.Username, "admin", user.IsAdmin())
	}
	if user.Disabled {
		// Disabling an account in the console has to outrank the identity
		// provider, or there is no way to revoke someone without also
		// removing them from the Access policy.
		writeError(w, http.StatusForbidden, "account disabled")
		return
	}
	// The admin list is re-applied on every sign-in rather than only at
	// creation: its whole job is making operator lockout unreachable, and
	// a rescue that only worked on a brand-new account would not do it.
	if !user.IsAdmin() && s.isAccessAdmin(user.Username) {
		if err := s.store.UpdateUser(r.Context(), user.ID, store.RoleAdmin, user.Permissions, false); err != nil {
			s.logger.Error("cloudflare access: promoting admin", "user", user.Username, "error", err)
		} else {
			user.Role = store.RoleAdmin
			s.logger.Info("cloudflare access: admin role applied from CF_ACCESS_ADMIN_EMAILS", "user", user.Username)
		}
	}

	token, err := s.signSession(user, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"username": user.Username,
		"created":  created,
	})
}

// isAccessAdmin reports whether an address is on the configured
// admin list.
func (s *Server) isAccessAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, e := range s.AccessAdminEmails {
		if e == email {
			return true
		}
	}
	return false
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleMe tells the frontend who it is and what it may do, so the UI can
// hide controls the server would reject anyway.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	perms := user.Permissions
	if user.IsAdmin() {
		perms = store.AllPermissions
	}
	if perms == nil {
		perms = []string{}
	}
	out := map[string]any{
		"username":    user.Username,
		"role":        user.Role,
		"isAdmin":     user.IsAdmin(),
		"permissions": perms,
	}
	// Signing out of a session that began at Access has to end the Access
	// session too, and only the frontend can send the browser there.
	if s.Access != nil && sessionIsSSO(r.Context()) {
		out["sso"] = true
		out["ssoLogoutURL"] = s.Access.LogoutURL()
	}
	writeJSON(w, http.StatusOK, out)
}

// BootstrapAdmin creates the initial admin user from ADMIN_USERNAME/
// ADMIN_PASSWORD if the users table is still empty. Safe to call on every
// startup; it's a no-op once any user exists.
func BootstrapAdmin(ctx context.Context, s *store.Store, username, password string) error {
	count, err := s.CountUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if password == "" {
		return errBootstrapPasswordRequired
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.CreateUser(ctx, username, string(hash), store.RoleAdmin, nil)
	return err
}
