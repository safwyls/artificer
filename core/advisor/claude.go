package advisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude answers advisor chats through the Anthropic API.
type Claude struct {
	prompt Prompt
	client anthropic.Client
	model  string
}

func NewClaude(apiKey, model string, prompt Prompt) *Claude {
	if model == "" {
		model = DefaultModel(ProviderAnthropic)
	}
	return &Claude{prompt: prompt, client: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model}
}

// Provider names where questions go, so the UI can say so honestly.
func (c *Claude) Provider() string { return "anthropic" }

func (c *Claude) Model() string { return c.model }

// claudeTools translates the browser's tool definitions. The schema arrives
// as raw JSON; properties and required are lifted out because that's how
// the SDK's param wants them.
func claudeTools(tools []Tool) ([]anthropic.BetaToolUnionParam, error) {
	out := make([]anthropic.BetaToolUnionParam, 0, len(tools))
	for _, t := range tools {
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("tool %s: invalid input schema: %w", t.Name, err)
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.BetaToolInputSchemaParam{
				Properties: schema.Properties,
				Required:   schema.Required,
			},
		}})
	}
	return out, nil
}

func claudeMessages(history []Message) ([]anthropic.BetaMessageParam, error) {
	msgs := make([]anthropic.BetaMessageParam, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case "user":
			msgs = append(msgs, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(m.Content)))
		case "assistant":
			blocks := make([]anthropic.BetaContentBlockParamUnion, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(m.Content))
			}
			// Echoed tool_use blocks must keep their ids — the provider
			// pairs them with the results in the next turn.
			for _, call := range m.ToolCalls {
				var input any
				if err := json.Unmarshal(call.Args, &input); err != nil {
					return nil, fmt.Errorf("tool call %s: invalid args: %w", call.Name, err)
				}
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfToolUse: &anthropic.BetaToolUseBlockParam{ID: call.ID, Name: call.Name, Input: input},
				})
			}
			msgs = append(msgs, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: blocks,
			})
		case "tool":
			blocks := make([]anthropic.BetaContentBlockParamUnion, 0, 1+len(m.ToolResults))
			for _, res := range m.ToolResults {
				block := anthropic.NewBetaToolResultTextBlockParam(res.ID, res.Content, false)
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{OfToolResult: &block})
			}
			// A note riding with the results — the browser's "last round,
			// answer now" nudge.
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(m.Content))
			}
			msgs = append(msgs, anthropic.NewBetaUserMessage(blocks...))
		default:
			return nil, fmt.Errorf("invalid message role %q", m.Role)
		}
	}
	return msgs, nil
}

// Chat sends one model turn and returns either final text or the tool calls
// the browser should run before asking again.
func (c *Claude) Chat(ctx context.Context, asker, gameContext string, tools []Tool, history []Message) (Reply, error) {
	msgs, err := claudeMessages(history)
	if err != nil {
		return Reply{}, err
	}
	toolParams, err := claudeTools(tools)
	if err != nil {
		return Reply{}, err
	}

	resp, err := c.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model: anthropic.Model(c.model),
		// Caps thinking plus reply together (thinking is on by default on
		// this model), so most of the room is for reasoning over a large
		// roster, not for the reply — the prompt asks for brevity.
		MaxTokens: 8192,
		System: []anthropic.BetaTextBlockParam{
			{Text: c.prompt.System},
			{Text: askerBlock(c.prompt.Console, asker) + "\n<server_data>\n" + gameContext + "\n</server_data>"},
		},
		Messages: msgs,
		Tools:    toolParams,
		// Safety classifiers can decline a request outright; "default"
		// fallbacks re-run it server-side on Anthropic's recommended
		// substitute (picked per refusal category) instead of failing the
		// chat — and leave no pinned fallback model to migrate later.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	})
	if err != nil {
		// 429 is a quota/rate hit, 529 the API overloaded — both "wait, or
		// fix the plan", which the key's owner should hear as such.
		var apierr *anthropic.Error
		if errors.As(err, &apierr) && (apierr.StatusCode == 429 || apierr.StatusCode == 529) {
			return Reply{}, rateLimited(err.Error())
		}
		return Reply{}, err
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return Reply{}, ErrRefused
	}

	var reply Reply
	var sb strings.Builder
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			sb.WriteString(b.Text)
		case anthropic.BetaToolUseBlock:
			args, err := json.Marshal(b.Input)
			if err != nil {
				return Reply{}, fmt.Errorf("tool call %s: %w", b.Name, err)
			}
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Args: args})
		}
	}
	reply.Text = strings.TrimSpace(sb.String())
	if reply.Text == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, errors.New("the advisor returned an empty reply")
	}
	return reply, nil
}
