package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/wildskeeper/internal/advisor"
)

// fakeAdvisor answers without a network, recording what the handler passed.
type fakeAdvisor struct {
	gotAsker   string
	gotContext string
	gotTools   []advisor.Tool
	gotHistory []advisor.Message
	reply      string
	toolCalls  []advisor.ToolCall
	err        error
}

func (f *fakeAdvisor) Provider() string { return "testprovider" }

func (f *fakeAdvisor) Model() string { return "test-model" }

func (f *fakeAdvisor) Chat(_ context.Context, asker, gameContext string, tools []advisor.Tool, history []advisor.Message) (advisor.Reply, error) {
	f.gotAsker = asker
	f.gotContext = gameContext
	f.gotTools = tools
	f.gotHistory = history
	if f.err != nil {
		return advisor.Reply{}, f.err
	}
	return advisor.Reply{Text: f.reply, ToolCalls: f.toolCalls}, nil
}

func createAdvisorServer(t *testing.T, app *testApp, admin []*http.Cookie) int64 {
	t.Helper()
	rec := app.do(t, "POST", "/api/servers", map[string]any{
		"name": "main", "host": "10.0.0.5", "rconPort": 25575, "restPort": 8212, "enabled": true,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server: got %d (body %s)", rec.Code, rec.Body)
	}
	return int64(decodeMap(t, rec)["id"].(float64))
}

func TestAdvisorStatusReportsConfiguration(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)

	// Without a key the feature is absent, and the frontend must be told so.
	rec := app.do(t, "GET", fmt.Sprintf("/api/servers/%d/advisor", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d (body %s)", rec.Code, rec.Body)
	}
	if enabled := decodeMap(t, rec)["enabled"]; enabled != false {
		t.Errorf("without advisor: enabled = %v, want false", enabled)
	}

	app.api.SetEnvAdvisor(&fakeAdvisor{reply: "ok"})
	rec = app.do(t, "GET", fmt.Sprintf("/api/servers/%d/advisor", id), nil, admin)
	status := decodeMap(t, rec)
	if status["enabled"] != true {
		t.Errorf("with advisor: enabled = %v, want true", status["enabled"])
	}
	// The UI names whose API the roster goes to; the name comes from here.
	if status["provider"] != "testprovider" {
		t.Errorf("provider = %v, want testprovider", status["provider"])
	}
}

func TestAdvisorChat(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	path := fmt.Sprintf("/api/servers/%d/advisor", id)

	// Unauthenticated requests never reach the handler.
	if rec := app.do(t, "POST", path, map[string]any{}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: got %d, want 401", rec.Code)
	}

	// Unconfigured: a clear refusal, not a crash on the nil client.
	rec := app.do(t, "POST", path, map[string]any{
		"context": "{}", "messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unconfigured: got %d, want 400 (body %s)", rec.Code, rec.Body)
	}

	fake := &fakeAdvisor{reply: "Condense your spare Pengullets."}
	app.api.SetEnvAdvisor(fake)

	rec = app.do(t, "POST", path, map[string]any{
		"context": `{"bases":[]}`,
		"messages": []map[string]string{
			{"role": "user", "content": "What should I condense?"},
		},
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat: got %d (body %s)", rec.Code, rec.Body)
	}
	if reply := decodeMap(t, rec)["reply"]; reply != "Condense your spare Pengullets." {
		t.Errorf("reply = %q", reply)
	}
	if fake.gotContext != `{"bases":[]}` {
		t.Errorf("context passed = %q", fake.gotContext)
	}
	if len(fake.gotHistory) != 1 || fake.gotHistory[0].Content != "What should I condense?" {
		t.Errorf("history passed = %+v", fake.gotHistory)
	}
	// The asker rides in from the session so the model can guess which
	// in-game player it's advising.
	if fake.gotAsker != adminName {
		t.Errorf("asker = %q, want %q", fake.gotAsker, adminName)
	}
}

func TestPersonalAdvisorKey(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.api.SetEnvAdvisor(&fakeAdvisor{reply: "ok"})
	app.createUser(t, admin, "player", "playerpass123", "user", nil)
	player := app.login(t, "player", "playerpass123")
	statusPath := fmt.Sprintf("/api/servers/%d/advisor", id)

	// A personal key shadows the shared client for its owner only.
	rec := app.do(t, "PUT", "/api/me/advisor-key", map[string]string{"provider": "gemini", "apiKey": "AIza-test"}, player)
	if rec.Code != http.StatusOK {
		t.Fatalf("set personal key: got %d (body %s)", rec.Code, rec.Body)
	}
	status := decodeMap(t, rec)
	if status["enabled"] != true || status["provider"] != "gemini" || status["source"] != "personal" || status["hasPersonalKey"] != true {
		t.Errorf("owner status after set: %+v", status)
	}
	// No model chosen → the provider default, resolved server-side.
	if status["model"] != "gemini-3.5-flash" {
		t.Errorf("default model = %v, want gemini-3.5-flash", status["model"])
	}

	// An explicit model choice sticks; one off the curated list is refused.
	rec = app.do(t, "PUT", "/api/me/advisor-key", map[string]string{"provider": "gemini", "apiKey": "AIza-test", "model": "gemini-3.6-flash"}, player)
	if rec.Code != http.StatusOK {
		t.Fatalf("set key with model: got %d (body %s)", rec.Code, rec.Body)
	}
	if status := decodeMap(t, rec); status["model"] != "gemini-3.6-flash" {
		t.Errorf("chosen model = %v, want gemini-3.6-flash", status["model"])
	}
	if rec := app.do(t, "PUT", "/api/me/advisor-key", map[string]string{"provider": "gemini", "apiKey": "k", "model": "gpt-4"}, player); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown model: got %d, want 400", rec.Code)
	}

	// The other account still rides the shared client.
	status = decodeMap(t, app.do(t, "GET", statusPath, nil, admin))
	if status["source"] != "env" || status["provider"] != "testprovider" || status["hasPersonalKey"] != false {
		t.Errorf("other user's status: %+v", status)
	}

	// Validation matches the shared endpoint's.
	if rec := app.do(t, "PUT", "/api/me/advisor-key", map[string]string{"provider": "openai", "apiKey": "k"}, player); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown provider: got %d, want 400", rec.Code)
	}

	// Removing it falls back to the shared client.
	status = decodeMap(t, app.do(t, "DELETE", "/api/me/advisor-key", nil, player))
	if status["source"] != "env" || status["hasPersonalKey"] != false {
		t.Errorf("owner status after delete: %+v", status)
	}
}

func TestAdvisorModelChange(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.createUser(t, admin, "player", "playerpass123", "user", nil)
	player := app.login(t, "player", "playerpass123")
	statusPath := fmt.Sprintf("/api/servers/%d/advisor", id)

	// Personal: no key yet → nothing to change.
	if rec := app.do(t, "PUT", "/api/me/advisor-key/model", map[string]string{"model": "gemini-3.6-flash"}, player); rec.Code != http.StatusBadRequest {
		t.Errorf("model change without key: got %d, want 400", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/me/advisor-key", map[string]string{"provider": "gemini", "apiKey": "AIza-test"}, player); rec.Code != http.StatusOK {
		t.Fatalf("set personal key: got %d (body %s)", rec.Code, rec.Body)
	}
	// The change sticks without re-entering the key, and cross-provider
	// models are refused against the key's own provider.
	status := decodeMap(t, app.do(t, "PUT", "/api/me/advisor-key/model", map[string]string{"model": "gemini-3.6-flash"}, player))
	if status["model"] != "gemini-3.6-flash" || status["provider"] != "gemini" {
		t.Errorf("after personal model change: %+v", status)
	}
	if rec := app.do(t, "PUT", "/api/me/advisor-key/model", map[string]string{"model": "claude-opus-5"}, player); rec.Code != http.StatusBadRequest {
		t.Errorf("cross-provider model: got %d, want 400", rec.Code)
	}

	// Shared: admin-only, and only for a UI-saved key (env has no row).
	if rec := app.do(t, "PUT", "/api/advisor/key/model", map[string]string{"model": "claude-sonnet-5"}, player); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin shared model change: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/advisor/key/model", map[string]string{"model": "claude-sonnet-5"}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("shared model change without saved key: got %d, want 400", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/advisor/key", map[string]string{"provider": "anthropic", "apiKey": "sk-test"}, admin); rec.Code != http.StatusOK {
		t.Fatalf("set shared key: got %d (body %s)", rec.Code, rec.Body)
	}
	status = decodeMap(t, app.do(t, "PUT", "/api/advisor/key/model", map[string]string{"model": "claude-sonnet-5"}, admin))
	if status["model"] != "claude-sonnet-5" {
		t.Errorf("after shared model change: %+v", status)
	}
	// The swapped-in client serves everyone without that personal key.
	status = decodeMap(t, app.do(t, "GET", statusPath, nil, admin))
	if status["model"] != "claude-sonnet-5" || status["source"] != "ui" {
		t.Errorf("admin status after shared change: %+v", status)
	}
}

func TestAdvisorChatValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.api.SetEnvAdvisor(&fakeAdvisor{reply: "ok"})
	path := fmt.Sprintf("/api/servers/%d/advisor", id)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no messages", map[string]any{"context": "{}", "messages": []map[string]string{}}},
		{"bad role", map[string]any{"context": "{}", "messages": []map[string]string{{"role": "system", "content": "x"}}}},
		{"empty content", map[string]any{"context": "{}", "messages": []map[string]string{{"role": "user", "content": ""}}}},
		{"oversized message", map[string]any{"context": "{}", "messages": []map[string]string{{"role": "user", "content": strings.Repeat("x", 5000)}}}},
	}
	for _, tc := range cases {
		if rec := app.do(t, "POST", path, tc.body, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 (body %s)", tc.name, rec.Code, rec.Body)
		}
	}
}

func TestAdvisorChatUpstreamFailures(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	path := fmt.Sprintf("/api/servers/%d/advisor", id)
	body := map[string]any{"context": "{}", "messages": []map[string]string{{"role": "user", "content": "hi"}}}

	// A refusal is the model's answer, not an outage — its own status so the
	// UI can show the message rather than a generic "unavailable".
	app.api.SetEnvAdvisor(&fakeAdvisor{err: advisor.ErrRefused})
	if rec := app.do(t, "POST", path, body, admin); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("refusal: got %d, want 422 (body %s)", rec.Code, rec.Body)
	}

	// A quota hit names itself, the wait, and (implicitly) whose key —
	// the one upstream failure a player can actually act on.
	app.api.SetEnvAdvisor(&fakeAdvisor{err: &advisor.RateLimitedError{RetryAfter: 57 * time.Second}})
	rec := app.do(t, "POST", path, body, admin)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("rate limited: got %d, want 429 (body %s)", rec.Code, rec.Body)
	}
	if msg, _ := decodeMap(t, rec)["error"].(string); !strings.Contains(msg, "usage limit") || !strings.Contains(msg, "57s") {
		t.Errorf("rate limit message = %q, want usage limit + wait", msg)
	}

	app.api.SetEnvAdvisor(&fakeAdvisor{err: fmt.Errorf("api timeout")})
	rec = app.do(t, "POST", path, body, admin)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("upstream error: got %d, want 502 (body %s)", rec.Code, rec.Body)
	}
	// The upstream error text may name internals; the client gets a fixed line.
	if msg := decodeMap(t, rec)["error"]; msg == "api timeout" {
		t.Error("upstream error text leaked to the client")
	}
}

func TestAdvisorChatToolLoop(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	path := fmt.Sprintf("/api/servers/%d/advisor", id)

	// Round one: the model asks for a tool call; the handler relays it to
	// the browser instead of a reply, echoing the ids the provider needs.
	fake := &fakeAdvisor{toolCalls: []advisor.ToolCall{
		{ID: "call-1", Name: "breed_child", Args: json.RawMessage(`{"parentA":"Frostallion","parentB":"Helzephyr"}`),
			// Gemini's thought signature must survive the browser round
			// trip byte-for-byte or the follow-up request 400s.
			Signature: []byte("sig-bytes")},
	}}
	app.api.SetEnvAdvisor(fake)
	tools := []map[string]any{{
		"name": "breed_child", "description": "Child species of a pair",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	}}
	rec := app.do(t, "POST", path, map[string]any{
		"context": "{}", "tools": tools,
		"messages": []map[string]any{{"role": "user", "content": "What do I breed for Frostallion Noct?"}},
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("tool round: got %d (body %s)", rec.Code, rec.Body)
	}
	round := decodeMap(t, rec)
	calls, ok := round["toolCalls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("toolCalls = %v", round["toolCalls"])
	}
	if sig := calls[0].(map[string]any)["signature"]; sig != "c2lnLWJ5dGVz" { // base64("sig-bytes")
		t.Errorf("signature = %v, want base64 of sig-bytes", sig)
	}
	if len(fake.gotTools) != 1 || fake.gotTools[0].Name != "breed_child" {
		t.Errorf("tools passed = %+v", fake.gotTools)
	}

	// Round two: the browser sends back the assistant turn and the tool
	// result; the handler accepts both roles and the fake sees the history.
	fake.toolCalls = nil
	fake.reply = "Breed Frostallion with Helzephyr."
	rec = app.do(t, "POST", path, map[string]any{
		"context": "{}", "tools": tools,
		"messages": []map[string]any{
			{"role": "user", "content": "What do I breed for Frostallion Noct?"},
			{"role": "assistant", "toolCalls": []map[string]any{{"id": "call-1", "name": "breed_child", "args": map[string]any{"parentA": "Frostallion", "parentB": "Helzephyr"}, "signature": "c2lnLWJ5dGVz"}}},
			{"role": "tool", "toolResults": []map[string]any{{"id": "call-1", "name": "breed_child", "content": "Frostallion x Helzephyr -> Frostallion Noct (special combo)"}}},
		},
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("final round: got %d (body %s)", rec.Code, rec.Body)
	}
	if reply := decodeMap(t, rec)["reply"]; reply != "Breed Frostallion with Helzephyr." {
		t.Errorf("reply = %q", reply)
	}
	if len(fake.gotHistory) != 3 || fake.gotHistory[2].Role != "tool" {
		t.Errorf("history passed = %+v", fake.gotHistory)
	}
	if sig := string(fake.gotHistory[1].ToolCalls[0].Signature); sig != "sig-bytes" {
		t.Errorf("echoed signature = %q, want sig-bytes", sig)
	}

	// An assistant turn with neither text nor calls is the one shape no
	// provider can translate.
	rec = app.do(t, "POST", path, map[string]any{
		"context": "{}",
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
			{"role": "assistant"},
		},
	}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty assistant turn: got %d, want 400 (body %s)", rec.Code, rec.Body)
	}
}

func TestDocsEndpoint(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	if rec := app.do(t, "GET", "/api/docs", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated docs: got %d, want 401", rec.Code)
	}
	rec := app.do(t, "GET", "/api/docs", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("docs: got %d (body %s)", rec.Code, rec.Body)
	}
	docs, ok := decodeMap(t, rec)["docs"].([]any)
	if !ok || len(docs) == 0 {
		t.Fatal("docs payload empty")
	}
	found := false
	for _, d := range docs {
		m := d.(map[string]any)
		if m["name"] == "architecture.md" && len(m["content"].(string)) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("architecture.md missing from docs payload")
	}
}

func TestAdvisorKeyManagement(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.createUser(t, admin, "player", "playerpass123", "user", nil)
	player := app.login(t, "player", "playerpass123")
	statusPath := fmt.Sprintf("/api/servers/%d/advisor", id)

	// The env-configured client is the fallback the saved key must beat.
	app.api.SetEnvAdvisor(&fakeAdvisor{reply: "ok"})

	// Key management is admin territory; validation names its limits.
	if rec := app.do(t, "PUT", "/api/advisor/key", map[string]string{"provider": "anthropic", "apiKey": "k"}, player); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin set key: got %d, want 403", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/advisor/key", map[string]string{"provider": "openai", "apiKey": "k"}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown provider: got %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if rec := app.do(t, "PUT", "/api/advisor/key", map[string]string{"provider": "anthropic", "apiKey": ""}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("empty key: got %d, want 400", rec.Code)
	}

	rec := app.do(t, "PUT", "/api/advisor/key", map[string]string{"provider": "anthropic", "apiKey": "sk-test"}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set key: got %d (body %s)", rec.Code, rec.Body)
	}
	status := decodeMap(t, rec)
	if status["enabled"] != true || status["provider"] != "anthropic" || status["source"] != "ui" {
		t.Errorf("after set: %+v, want enabled/anthropic/ui", status)
	}

	// The saved key wins over the env client, and survives a "restart".
	if got, err := app.api.LoadStoredAdvisor(context.Background()); err != nil || got != "anthropic" {
		t.Errorf("reload stored key: provider %q, err %v", got, err)
	}

	// Non-admins see availability but not the configure affordance, and the
	// key itself appears nowhere in the payload.
	status = decodeMap(t, app.do(t, "GET", statusPath, nil, player))
	if status["canConfigure"] != false || status["provider"] != "anthropic" {
		t.Errorf("player status: %+v", status)
	}
	if _, leaked := status["apiKey"]; leaked {
		t.Error("status payload carries the key")
	}

	// Removing the saved key falls back to the environment client.
	status = decodeMap(t, app.do(t, "DELETE", "/api/advisor/key", nil, admin))
	if status["enabled"] != true || status["provider"] != "testprovider" || status["source"] != "env" {
		t.Errorf("after delete: %+v, want enabled/testprovider/env", status)
	}
}

func TestAdvisorSettings(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.createUser(t, admin, "player", "playerpass123", "user", nil)
	player := app.login(t, "player", "playerpass123")
	statusPath := fmt.Sprintf("/api/servers/%d/advisor", id)

	// The default cap rides the status before anything is stored.
	if status := decodeMap(t, app.do(t, "GET", statusPath, nil, admin)); status["maxToolRounds"] != float64(8) {
		t.Errorf("default maxToolRounds = %v, want 8", status["maxToolRounds"])
	}

	if rec := app.do(t, "PUT", "/api/advisor/settings", map[string]int{"maxToolRounds": 12}, player); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin set settings: got %d, want 403", rec.Code)
	}
	for _, bad := range []int{0, 25} {
		if rec := app.do(t, "PUT", "/api/advisor/settings", map[string]int{"maxToolRounds": bad}, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("maxToolRounds=%d: got %d, want 400", bad, rec.Code)
		}
	}

	rec := app.do(t, "PUT", "/api/advisor/settings", map[string]int{"maxToolRounds": 12}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set settings: got %d (body %s)", rec.Code, rec.Body)
	}
	if status := decodeMap(t, rec); status["maxToolRounds"] != float64(12) {
		t.Errorf("after set: maxToolRounds = %v, want 12", status["maxToolRounds"])
	}
	// The cap is server-wide: every user's loop honors it.
	if status := decodeMap(t, app.do(t, "GET", statusPath, nil, player)); status["maxToolRounds"] != float64(12) {
		t.Errorf("player sees maxToolRounds = %v, want 12", status["maxToolRounds"])
	}
}

func TestAdvisorChatHonorsFeatureHide(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createAdvisorServer(t, app, admin)
	app.api.SetEnvAdvisor(&fakeAdvisor{reply: "ok"})
	app.createUser(t, admin, "player", "playerpass123", "user", nil)
	player := app.login(t, "player", "playerpass123")

	rec := app.do(t, "PUT", fmt.Sprintf("/api/servers/%d/visibility", id), map[string]any{
		"hiddenFeatures": []string{"calculators"}, "players": map[string][]string{},
	}, admin)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("hide calculators: got %d (body %s)", rec.Code, rec.Body)
	}

	body := map[string]any{"context": "{}", "messages": []map[string]string{{"role": "user", "content": "hi"}}}
	if rec := app.do(t, "POST", fmt.Sprintf("/api/servers/%d/advisor", id), body, player); rec.Code != http.StatusForbidden {
		t.Errorf("hidden calculators: got %d, want 403 (body %s)", rec.Code, rec.Body)
	}
}
