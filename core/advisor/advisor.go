// Package advisor answers natural-language questions about a server's pals
// and base crews through a hosted model API — Anthropic's Claude
// (claude.go) or Google's Gemini (gemini.go), whichever key the operator
// configured. Both speak the same Chat shape, so the HTTP handler and the
// frontend never know which one is behind it.
//
// The split with the frontend is deliberate: every derived number
// (effective work levels, condenser math, stat estimates) lives in the
// browser's calculators, and the vendored game catalogs never made it into
// the Go binary. So the browser builds a compact JSON summary of what it
// already computed and sends it with each question; this package only wraps
// it in a system prompt that explains the schema and the game's mechanics,
// and keeps the API key on the server where the browser can never see it.
package advisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Providers, as spelled in config, the store, and the status endpoint.
const (
	ProviderAnthropic = "anthropic"
	ProviderGemini    = "gemini"
)

// ModelOption is one model a key's owner may pick. A curated list rather
// than free text: a mistyped model ID would sit in the store and 404 on
// every question. The first entry per provider is the default.
type ModelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ModelOptions rides the status payload so the UI's picker and this
// package's validation can never drift. Kept to generally-available models
// on purpose — preview IDs get retired on a schedule and take the advisor
// down with them (gemini-3-pro-preview did exactly that, 2026-03).
var ModelOptions = map[string][]ModelOption{
	ProviderAnthropic: {
		{ID: "claude-opus-5", Label: "Claude Opus 5 · most capable"},
		{ID: "claude-sonnet-5", Label: "Claude Sonnet 5 · balanced"},
		{ID: "claude-haiku-4-5", Label: "Claude Haiku 4.5 · fastest"},
	},
	ProviderGemini: {
		{ID: "gemini-3.5-flash", Label: "Gemini 3.5 Flash · most capable"},
		{ID: "gemini-3.6-flash", Label: "Gemini 3.6 Flash · efficient"},
		{ID: "gemini-3.5-flash-lite", Label: "Gemini 3.5 Flash-Lite · fastest"},
	},
}

// DefaultModel is what an empty model choice (older stored keys, env keys)
// resolves to.
func DefaultModel(provider string) string {
	if opts := ModelOptions[provider]; len(opts) > 0 {
		return opts[0].ID
	}
	return ""
}

// ValidModel reports whether a stored choice is one this package will run.
// Empty is valid — it means the provider default.
func ValidModel(provider, model string) bool {
	if model == "" {
		return true
	}
	for _, opt := range ModelOptions[provider] {
		if opt.ID == model {
			return true
		}
	}
	return false
}

// New builds the client for a provider name — the one place the string
// becomes a concrete implementation, shared by env wiring and the UI-key
// endpoint so the two can never drift on which names are valid. model may
// be empty (the provider default) but must otherwise be a ModelOptions ID.
func New(ctx context.Context, provider, apiKey, model string, prompt Prompt) (Client, error) {
	if !ValidModel(provider, model) {
		return nil, fmt.Errorf("unknown model %q for provider %q", model, provider)
	}
	switch provider {
	case ProviderAnthropic:
		return NewClaude(apiKey, model, prompt), nil
	case ProviderGemini:
		return NewGemini(ctx, apiKey, model, prompt)
	default:
		return nil, fmt.Errorf("unknown advisor provider %q", provider)
	}
}

// Client is what both providers implement; the API layer's AdvisorClient
// interface matches it structurally.
type Client interface {
	// Chat sends one model turn. asker is the signed-in username —
	// injected server-side so the model's "who am I advising" guess works
	// from a name the browser can't spoof.
	Chat(ctx context.Context, asker, gameContext string, tools []Tool, history []Message) (Reply, error)
	Provider() string
	// Model is the resolved model ID questions run on, for the status UI.
	Model() string
}

// askerBlock renders the identity line both providers append to the system
// context. Empty asker (shouldn't happen behind auth) renders nothing.
func askerBlock(console, asker string) string {
	if asker == "" {
		return ""
	}
	return fmt.Sprintf("The person asking is signed in to %s with the username %q.", console, asker)
}

// Message is one prior turn of the conversation, resent in full on every
// request — the server stores nothing between questions.
//
// The tool fields exist because the calculators the advisor can call live
// in the browser, not here: the model asks for a call (an assistant turn
// carrying toolCalls), the browser runs the real calculator and answers (a
// "tool" turn carrying toolResults), and the loop driver is the browser.
// This package only translates the exchange into each provider's wire
// format.
type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content,omitempty"`
	ToolCalls   []ToolCall   `json:"toolCalls,omitempty"`
	ToolResults []ToolResult `json:"toolResults,omitempty"`
}

// Tool is a browser-implemented function the model may call. Definitions
// ride in from the browser with each request — the single source of truth
// stays next to the code that executes them (web/src/lib/advisor-tools.ts),
// and the server never needs to know what a breeding table is.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCall is the model asking the browser to run one tool.
type ToolCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
	// Signature is Gemini's thought signature for the call — opaque bytes
	// (base64 on the wire) the provider requires echoed back verbatim with
	// the call, or the next request 400s. The browser never reads it, it
	// just rides the round trip; Claude has no equivalent and ignores it.
	Signature []byte `json:"signature,omitempty"`
}

// ToolResult is the browser's answer to one call.
type ToolResult struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Reply is one model turn: final text, or a request for tool calls (Text
// may carry preamble either way).
type Reply struct {
	Text      string     `json:"text"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
}

// ErrRefused reports that the model declined to answer. Its own message when
// surfaced to the user, rather than a generic upstream error, because a
// retry won't help and the question is what has to change.
var ErrRefused = errors.New("the advisor declined to answer this question")

// RateLimitedError reports the provider refusing for quota or rate reasons.
// Its own type rather than a generic upstream error because it's actionable
// for whoever owns the key — wait it out, or fix the plan — and the handler
// words the message differently for a personal key than a shared one.
type RateLimitedError struct {
	// RetryAfter is the provider's suggested wait, 0 when it named none.
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited, retry in %s", e.RetryAfter.Round(time.Second))
	}
	return "rate limited"
}

// Both providers spell the suggested wait inside the message text ("Please
// retry in 57.05s") more reliably than in typed fields; scrape it, treat a
// miss as unknown.
var retryAfterRe = regexp.MustCompile(`(?i)retry in ([0-9]+(?:\.[0-9]+)?)\s*s`)

func rateLimited(message string) *RateLimitedError {
	if m := retryAfterRe.FindStringSubmatch(message); m != nil {
		if secs, err := strconv.ParseFloat(m[1], 64); err == nil {
			return &RateLimitedError{RetryAfter: time.Duration(secs * float64(time.Second))}
		}
	}
	return &RateLimitedError{}
}

// Prompt is the advisor's system prompt, supplied by the console's
// wiring: the console name (for the asker line and the UI), and the full
// system text — role, grounding order, the game's <server_data> shape and
// mechanics, tool guidance, and answer style. The text is game payload,
// so it lives with the game module (palcon's Palworld prompt is the
// original); this package only carries it to the providers, verbatim and
// shared between them so a server switching providers gets the same kind
// of advice.
type Prompt struct {
	// Console is the console's display name ("palcon").
	Console string
	// System is the full system-prompt text.
	System string
}
