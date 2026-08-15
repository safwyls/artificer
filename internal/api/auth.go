package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/safwyls/flamekeeper/internal/store"
)

const sessionCookieName = "flamekeeper_session"
const sessionDuration = 7 * 24 * time.Hour

type sessionClaims struct {
	// UserID, not the username: the request path reloads the user each
	// time so permission and password changes apply immediately, and a
	// rename doesn't invalidate a live session.
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (s *Server) signSession(user *store.User) (string, error) {
	claims := sessionClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
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

	token, err := s.signSession(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionDuration),
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"username":    user.Username,
		"role":        user.Role,
		"isAdmin":     user.IsAdmin(),
		"permissions": perms,
	})
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
