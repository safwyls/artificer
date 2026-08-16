package advisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// Gemini answers advisor chats through the Google Gemini API.
type Gemini struct {
	prompt Prompt
	client *genai.Client
	model  string
}

// NewGemini can fail (the client validates its configuration up front),
// unlike NewClaude — which is why main treats the two differently.
func NewGemini(ctx context.Context, apiKey, model string, prompt Prompt) (*Gemini, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	if model == "" {
		model = DefaultModel(ProviderGemini)
	}
	return &Gemini{prompt: prompt, client: client, model: model}, nil
}

// Provider names where questions go, so the UI can say so honestly.
func (g *Gemini) Provider() string { return "gemini" }

func (g *Gemini) Model() string { return g.model }

func geminiTools(tools []Tool) ([]*genai.Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		var schema any
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("tool %s: invalid input schema: %w", t.Name, err)
		}
		decls = append(decls, &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: schema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}, nil
}

func geminiContents(history []Message) ([]*genai.Content, error) {
	contents := make([]*genai.Content, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case "user":
			contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{{Text: m.Content}}})
		case "assistant":
			parts := make([]*genai.Part, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				parts = append(parts, &genai.Part{Text: m.Content})
			}
			// Echoed function calls must round-trip so the model can pair
			// them with the responses in the next content.
			for _, call := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(call.Args, &args); err != nil {
					return nil, fmt.Errorf("tool call %s: invalid args: %w", call.Name, err)
				}
				parts = append(parts, &genai.Part{
					FunctionCall:     &genai.FunctionCall{ID: call.ID, Name: call.Name, Args: args},
					ThoughtSignature: call.Signature,
				})
			}
			contents = append(contents, &genai.Content{Role: genai.RoleModel, Parts: parts})
		case "tool":
			parts := make([]*genai.Part, 0, 1+len(m.ToolResults))
			for _, res := range m.ToolResults {
				parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
					ID:       res.ID,
					Name:     res.Name,
					Response: map[string]any{"result": res.Content},
				}})
			}
			// A note riding with the results — the browser's "last round,
			// answer now" nudge.
			if m.Content != "" {
				parts = append(parts, &genai.Part{Text: m.Content})
			}
			contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: parts})
		default:
			return nil, fmt.Errorf("invalid message role %q", m.Role)
		}
	}
	return contents, nil
}

// Chat sends one model turn and returns either final text or the tool calls
// the browser should run before asking again.
func (g *Gemini) Chat(ctx context.Context, asker, gameContext string, tools []Tool, history []Message) (Reply, error) {
	contents, err := geminiContents(history)
	if err != nil {
		return Reply{}, err
	}
	toolDefs, err := geminiTools(tools)
	if err != nil {
		return Reply{}, err
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{
			{Text: g.prompt.System},
			{Text: askerBlock(g.prompt.Console, asker) + "\n<server_data>\n" + gameContext + "\n</server_data>"},
		}},
		Tools: toolDefs,
		// Reasoning tokens count against this too, so most of the room is
		// for thinking over a large roster — the prompt asks for brevity.
		MaxOutputTokens: 8192,
	})
	if err != nil {
		// Quota exhaustion (free tiers hit this fast) — actionable for the
		// key's owner, so it must not drown in "advisor unavailable".
		var apiErr genai.APIError
		if errors.As(err, &apiErr) && apiErr.Code == 429 {
			return Reply{}, rateLimited(err.Error())
		}
		return Reply{}, err
	}
	// A block can land on the prompt (nothing generated at all) or on the
	// candidate (generation cut off); both are the model declining, not an
	// outage, and map to the same error the handler turns into a 422.
	if fb := resp.PromptFeedback; fb != nil && fb.BlockReason != "" && fb.BlockReason != genai.BlockedReasonUnspecified {
		return Reply{}, ErrRefused
	}

	var reply Reply
	var sb strings.Builder
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		switch resp.Candidates[0].FinishReason {
		case genai.FinishReasonSafety, genai.FinishReasonProhibitedContent:
			return Reply{}, ErrRefused
		}
		for _, part := range resp.Candidates[0].Content.Parts {
			// Thought parts are the model's reasoning summary, not the
			// reply — the SDK's own Text() helper skips them, and walking
			// parts by hand (needed for the function calls) must too, or
			// the model's notes to itself leak into the visible answer.
			if part.Text != "" && !part.Thought {
				sb.WriteString(part.Text)
			}
			if fc := part.FunctionCall; fc != nil {
				args, err := json.Marshal(fc.Args)
				if err != nil {
					return Reply{}, fmt.Errorf("tool call %s: %w", fc.Name, err)
				}
				reply.ToolCalls = append(reply.ToolCalls, ToolCall{
					ID: fc.ID, Name: fc.Name, Args: args,
					Signature: part.ThoughtSignature,
				})
			}
		}
	}
	reply.Text = strings.TrimSpace(sb.String())
	if reply.Text == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, errors.New("the advisor returned an empty reply")
	}
	return reply, nil
}
