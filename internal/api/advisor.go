package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"time"

	"github.com/safwyls/dwcon/docs"
	"github.com/safwyls/dwcon/internal/advisor"
	"github.com/safwyls/dwcon/internal/store"
)

// handleDocs serves the embedded project documentation for the advisor's
// docs-search tool, which runs in the browser like the other tools. One
// blob, fetched once and cached client-side — the docs change only with
// the binary.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	entries, err := fs.Glob(docs.FS, "*.md")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read docs")
		return
	}
	sort.Strings(entries)
	type doc struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	out := make([]doc, 0, len(entries))
	for _, name := range entries {
		content, err := fs.ReadFile(docs.FS, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read docs")
			return
		}
		out = append(out, doc{Name: name, Content: string(content)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"docs": out})
}

// AdvisorClient is what the chat handler needs from the advisor — an
// interface rather than a concrete provider so main can wire Claude or
// Gemini (whichever key is set) and tests can answer without a network.
type AdvisorClient interface {
	Chat(ctx context.Context, asker, gameContext string, tools []advisor.Tool, history []advisor.Message) (advisor.Reply, error)
	// Provider names where questions go ("anthropic", "gemini"), so the UI
	// can tell players whose API their roster is sent to.
	Provider() string
	Model() string
}

// Conversations are bounded well below the 1 MiB body cap so a runaway
// client hits a named limit, not a generic decode failure. The context is
// the big part: the browser summarizes a whole server's pals into it.
const (
	advisorMaxTurns       = 40
	advisorMaxMessageSize = 4_000
	advisorMaxContextSize = 700_000
	advisorMaxTools       = 16
	advisorMaxToolPayload = 16_000
	// Assistant turns are the model's own output resent as history, so
	// their ceiling is what the model can produce (8192 output tokens),
	// not what a person types. Same for how many tool calls it fans out
	// in one turn — a dozen breed_child probes at once is normal work,
	// and the results turn must be allowed to answer every one of them.
	advisorMaxReplySize = 40_000
	advisorMaxToolCalls = 24
	// How many tool round-trips one question may take before the browser
	// tells the model to answer with what it has. The default suits broad
	// questions that probe several pairs sequentially; admins can tune it
	// (1-20; the 40-turn conversation cap is the hard ceiling behind it).
	advisorDefaultMaxRounds = 8
	advisorMaxRoundsCeiling = 20
)

// validAdvisorMessage checks one turn against the shape the providers can
// translate. Limits are per-field so a violation names itself.
func validAdvisorMessage(m advisor.Message) string {
	switch m.Role {
	case "user":
		if m.Content == "" || len(m.Content) > advisorMaxMessageSize {
			return "user messages must be between 1 and 4000 characters"
		}
	case "assistant":
		// An assistant turn may be pure text, pure tool calls, or both —
		// but never neither.
		if m.Content == "" && len(m.ToolCalls) == 0 {
			return "assistant messages need content or tool calls"
		}
		if len(m.Content) > advisorMaxReplySize || len(m.ToolCalls) > advisorMaxToolCalls {
			return "assistant message too large"
		}
		for _, c := range m.ToolCalls {
			if c.Name == "" || len(c.Name) > 64 || len(c.Args) > advisorMaxToolPayload || len(c.Signature) > advisorMaxToolPayload {
				return "tool call too large"
			}
		}
	case "tool":
		if len(m.ToolResults) == 0 || len(m.ToolResults) > advisorMaxToolCalls {
			return "tool messages must carry between 1 and 24 results"
		}
		// A tool turn may carry a short text alongside the results — the
		// browser's "last round, answer now" nudge rides there.
		if len(m.Content) > advisorMaxMessageSize {
			return "tool message note too large"
		}
		for _, r := range m.ToolResults {
			if r.Name == "" || len(r.Name) > 64 || len(r.Content) > advisorMaxToolPayload {
				return "tool result too large"
			}
		}
	default:
		return "message roles must be user, assistant or tool"
	}
	return ""
}

// SetEnvAdvisor wires the environment-configured client (main's job, like
// Provisioner). It is the fallback: a key saved through the UI wins.
func (s *Server) SetEnvAdvisor(c AdvisorClient) {
	s.advisorMu.Lock()
	defer s.advisorMu.Unlock()
	s.envAdvisor = c
}

// LoadStoredAdvisor builds the client for a key saved through the UI, if
// one is stored. Called once at startup; returns the provider name ("" when
// nothing is stored). An unusable stored key (rotated ENCRYPTION_KEY,
// provider removed) is an error for main to log, not to die on — the env
// fallback or plain absence both beat refusing to start.
func (s *Server) LoadStoredAdvisor(ctx context.Context) (string, error) {
	key, err := s.store.AdvisorKey(ctx)
	if err != nil || key == nil {
		return "", err
	}
	client, err := advisor.New(ctx, key.Provider, key.APIKey, key.Model)
	if err != nil {
		return "", err
	}
	s.advisorMu.Lock()
	defer s.advisorMu.Unlock()
	s.uiAdvisor = client
	return key.Provider, nil
}

// advisor resolves the shared client and where it came from: the UI-saved
// key first, then the environment, then absence.
func (s *Server) advisor() (client AdvisorClient, source string) {
	s.advisorMu.RLock()
	defer s.advisorMu.RUnlock()
	if s.uiAdvisor != nil {
		return s.uiAdvisor, "ui"
	}
	if s.envAdvisor != nil {
		return s.envAdvisor, "env"
	}
	return nil, ""
}

// advisorFor resolves the client for one request: the user's personal key
// shadows the shared client — their questions ride their own billing and
// never touch the admin's key. The error names an unusable personal key so
// the handler can tell the user to fix it rather than blaming the server.
func (s *Server) advisorFor(r *http.Request) (AdvisorClient, string, error) {
	if user, ok := userFromContext(r.Context()); ok {
		key, err := s.store.UserAdvisorKey(r.Context(), user.ID)
		if err != nil {
			// A corrupt row shouldn't lock someone out of the shared
			// client; log and fall through.
			s.logger.Error("reading personal advisor key", "user", user.ID, "error", err)
		} else if key != nil {
			// Built per request: construction is local validation only, no
			// network. Background context because the client outlives it.
			client, err := advisor.New(context.Background(), key.Provider, key.APIKey, key.Model)
			if err != nil {
				return nil, "", fmt.Errorf("your personal advisor key is unusable — replace or remove it")
			}
			return client, "personal", nil
		}
	}
	client, source := s.advisor()
	return client, source, nil
}

// advisorStatus is what GET /servers/{id}/advisor and every key endpoint
// return, so the frontend always holds one current, consistent picture —
// of THIS user's view: provider and source describe the client their
// questions would actually ride.
func (s *Server) advisorStatus(r *http.Request) map[string]any {
	client, source := s.advisor()
	canConfigure := false
	hasPersonalKey := false
	provider := ""
	model := ""
	enabled := client != nil
	if user, ok := userFromContext(r.Context()); ok {
		canConfigure = user.IsAdmin()
		if key, err := s.store.UserAdvisorKey(r.Context(), user.ID); err != nil {
			s.logger.Error("reading personal advisor key", "user", user.ID, "error", err)
		} else if key != nil {
			// Reported from the stored row, not a built client — an
			// unusable key still shows as theirs so they know to fix it.
			hasPersonalKey = true
			source = "personal"
			provider = key.Provider
			model = key.Model
			if model == "" {
				model = advisor.DefaultModel(key.Provider)
			}
			enabled = true
		}
	}
	if !hasPersonalKey && client != nil {
		provider = client.Provider()
		model = client.Model()
	}
	// The tool-round cap rides the status because the loop it bounds runs
	// in the browser.
	maxRounds, err := s.store.AdvisorMaxRounds(r.Context())
	if err != nil {
		s.logger.Error("reading advisor max rounds", "error", err)
	}
	if maxRounds <= 0 || maxRounds > advisorMaxRoundsCeiling {
		maxRounds = advisorDefaultMaxRounds
	}
	return map[string]any{
		"enabled":        enabled,
		"provider":       provider,
		"model":          model,
		"source":         source,
		"canConfigure":   canConfigure,
		"hasPersonalKey": hasPersonalKey,
		"maxToolRounds":  maxRounds,
		// The picker's choices — served rather than hardcoded client-side,
		// so the UI can never offer a model the server would reject.
		"modelOptions": advisor.ModelOptions,
	}
}

// handleSetAdvisorSettings stores the advisor's tuning knobs — currently
// just the tool-round cap. Admin-only, like the shared key.
func (s *Server) handleSetAdvisorSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxToolRounds int `json:"maxToolRounds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MaxToolRounds < 1 || req.MaxToolRounds > advisorMaxRoundsCeiling {
		writeError(w, http.StatusBadRequest, "maxToolRounds must be between 1 and 20")
		return
	}
	if err := s.store.SetAdvisorMaxRounds(r.Context(), req.MaxToolRounds); err != nil {
		s.logger.Error("saving advisor settings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleSetUserAdvisorKey stores the signed-in user's personal key —
// encrypted, never echoed back, and read only for their own requests.
func (s *Server) handleSetUserAdvisorKey(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.APIKey == "" || len(req.APIKey) > 512 {
		writeError(w, http.StatusBadRequest, "apiKey must be between 1 and 512 characters")
		return
	}
	// Build first, store second — same rule as the shared key.
	if _, err := advisor.New(context.Background(), req.Provider, req.APIKey, req.Model); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetUserAdvisorKey(r.Context(), user.ID, store.AdvisorKey{Provider: req.Provider, APIKey: req.APIKey, Model: req.Model}); err != nil {
		s.logger.Error("saving personal advisor key", "user", user.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save the key")
		return
	}
	s.logger.Info("personal advisor key saved", "user", user.ID, "provider", req.Provider)
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

func (s *Server) handleDeleteUserAdvisorKey(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := s.store.DeleteUserAdvisorKey(r.Context(), user.ID); err != nil {
		s.logger.Error("removing personal advisor key", "user", user.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove the key")
		return
	}
	s.logger.Info("personal advisor key removed", "user", user.ID)
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleAdvisorStatus tells the frontend whether the advisor is available,
// whose API answers it, and whether this user may configure it. Per-server
// only in its path — the capability is process-wide — but living under the
// server route keeps it next to the chat endpoint it gates.
func (s *Server) handleAdvisorStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.loadServer(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleSetAdvisorKey stores a model API key submitted through the admin
// UI — encrypted with the same box as server credentials, never echoed
// back — and swaps the live client so the change takes effect without a
// restart.
func (s *Server) handleSetAdvisorKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.APIKey == "" || len(req.APIKey) > 512 {
		writeError(w, http.StatusBadRequest, "apiKey must be between 1 and 512 characters")
		return
	}
	// Build first, store second: a bad provider or model never lands in
	// the database. The client is built on a background context because it
	// outlives this request.
	client, err := advisor.New(context.Background(), req.Provider, req.APIKey, req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetAdvisorKey(r.Context(), store.AdvisorKey{Provider: req.Provider, APIKey: req.APIKey, Model: req.Model}); err != nil {
		s.logger.Error("saving advisor key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save the key")
		return
	}
	s.advisorMu.Lock()
	s.uiAdvisor = client
	s.advisorMu.Unlock()
	s.logger.Info("advisor key saved", "provider", req.Provider)
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleSetAdvisorKeyModel changes which model the saved shared key runs,
// without re-entering the key. Only a UI-saved key can change — an env key
// has no stored row to update and runs the provider default.
func (s *Server) handleSetAdvisorKeyModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	key, err := s.store.AdvisorKey(r.Context())
	if err != nil {
		s.logger.Error("reading advisor key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read the saved key")
		return
	}
	if key == nil {
		writeError(w, http.StatusBadRequest, "no saved server key — a key from the environment runs the provider default")
		return
	}
	key.Model = req.Model
	client, err := advisor.New(context.Background(), key.Provider, key.APIKey, req.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetAdvisorKey(r.Context(), *key); err != nil {
		s.logger.Error("saving advisor key model", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save the model choice")
		return
	}
	s.advisorMu.Lock()
	s.uiAdvisor = client
	s.advisorMu.Unlock()
	s.logger.Info("advisor model changed", "provider", key.Provider, "model", req.Model)
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleSetUserAdvisorKeyModel is the same change for the signed-in user's
// personal key. No client swap — personal clients are built per request.
func (s *Server) handleSetUserAdvisorKeyModel(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	key, err := s.store.UserAdvisorKey(r.Context(), user.ID)
	if err != nil {
		s.logger.Error("reading personal advisor key", "user", user.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read your key")
		return
	}
	if key == nil {
		writeError(w, http.StatusBadRequest, "no personal key saved")
		return
	}
	if !advisor.ValidModel(key.Provider, req.Model) {
		writeError(w, http.StatusBadRequest, "unknown model for your key's provider")
		return
	}
	key.Model = req.Model
	if err := s.store.SetUserAdvisorKey(r.Context(), user.ID, *key); err != nil {
		s.logger.Error("saving personal advisor key model", "user", user.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save the model choice")
		return
	}
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleDeleteAdvisorKey removes the saved key. The advisor falls back to
// the environment-configured client, or to plain absence.
func (s *Server) handleDeleteAdvisorKey(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAdvisorKey(r.Context()); err != nil {
		s.logger.Error("removing advisor key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove the key")
		return
	}
	s.advisorMu.Lock()
	s.uiAdvisor = nil
	s.advisorMu.Unlock()
	s.logger.Info("advisor key removed")
	writeJSON(w, http.StatusOK, s.advisorStatus(r))
}

// handleAdvisorChat answers one question about the server's pals. The
// browser sends the full conversation and a JSON context it built from the
// same /pals payload it renders — so player-visibility hides are already
// applied, and the server never recomputes what the calculators know.
func (s *Server) handleAdvisorChat(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	// Rides the calculators view: the advisor reasons over the same derived
	// data that view shows, so switching the view off switches this off.
	if !requireFeature(w, r, srv, store.FeatureCalculators) {
		return
	}
	client, source, err := s.advisorFor(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if client == nil {
		writeError(w, http.StatusBadRequest, "advisor not configured")
		return
	}
	var req struct {
		Context  string            `json:"context"`
		Tools    []advisor.Tool    `json:"tools"`
		Messages []advisor.Message `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Messages) == 0 || len(req.Messages) > advisorMaxTurns {
		writeError(w, http.StatusBadRequest, "conversation must have between 1 and 40 messages")
		return
	}
	for _, m := range req.Messages {
		if msg := validAdvisorMessage(m); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if len(req.Tools) > advisorMaxTools {
		writeError(w, http.StatusBadRequest, "too many tools")
		return
	}
	for _, t := range req.Tools {
		if t.Name == "" || len(t.Name) > 64 || len(t.Description) > 2_000 || len(t.InputSchema) > 4_000 {
			writeError(w, http.StatusBadRequest, "tool definition too large")
			return
		}
	}
	if len(req.Context) > advisorMaxContextSize {
		writeError(w, http.StatusBadRequest, "context too large")
		return
	}

	// The asker's identity comes from the session, not the request body —
	// the model's "which player is this" guess starts from a name the
	// browser can't spoof.
	asker := ""
	if user, ok := userFromContext(r.Context()); ok {
		asker = user.Username
	}
	reply, err := client.Chat(r.Context(), asker, req.Context, req.Tools, req.Messages)
	if errors.Is(err, advisor.ErrRefused) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// A quota/rate hit is actionable — say whose key ran dry and how long
	// to wait, instead of a generic "unavailable". Expected operational
	// noise on free tiers, so logged as a warning, not an error.
	var limited *advisor.RateLimitedError
	if errors.As(err, &limited) {
		wait := " — try again in a minute"
		if limited.RetryAfter > 0 {
			wait = fmt.Sprintf(" — try again in about %s", limited.RetryAfter.Round(time.Second))
		}
		msg := "The server's advisor key has hit its usage limit" + wait +
			". If this keeps happening, an admin may need a higher quota with the provider."
		if source == "personal" {
			msg = "Your advisor key has hit its usage limit" + wait +
				". Check your plan and billing with the provider, or remove your key to use the server's."
		}
		s.logger.Warn("advisor rate limited", "server", srv.ID, "source", source, "retryAfter", limited.RetryAfter)
		writeError(w, http.StatusTooManyRequests, msg)
		return
	}
	if err != nil {
		s.logger.Error("advisor chat failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, "the advisor is unavailable right now")
		return
	}
	// Two shapes: tool calls to run (the browser executes them and asks
	// again), or the final reply. The browser tells them apart by which
	// field is present.
	if len(reply.ToolCalls) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"content": reply.Text, "toolCalls": reply.ToolCalls})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reply": reply.Text})
}
