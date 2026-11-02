package ai

import (
	"context"
	"fmt"
	"strings"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropic は Anthropic Messages API の実装。
type anthropic struct{ client }

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system"`
	Messages  []chatMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// GenerateReply は Provider の実装。
func (a *anthropic) GenerateReply(ctx context.Context, in ReplyInput) (string, error) {
	req := anthropicRequest{
		Model:     a.model,
		MaxTokens: anthropicMaxTokens,
		System:    systemPrompt(in),
		Messages:  []chatMessage{{Role: "user", Content: userPrompt(in)}},
	}
	headers := map[string]string{"x-api-key": a.apiKey, "anthropic-version": anthropicVersion}
	var resp anthropicResponse
	if err := a.postJSON(ctx, a.base+"/v1/messages", headers, req, &resp); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range resp.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return nonEmpty(b.String())
}

// openAI は OpenAI 互換 Chat Completions の実装。
type openAI struct{ client }

type openAIRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GenerateReply は Provider の実装。
func (o *openAI) GenerateReply(ctx context.Context, in ReplyInput) (string, error) {
	req := openAIRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt(in)},
			{Role: "user", Content: userPrompt(in)},
		},
	}
	headers := map[string]string{"Authorization": "Bearer " + o.apiKey}
	var resp openAIResponse
	if err := o.postJSON(ctx, o.base+"/chat/completions", headers, req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%w: 応答が空です", ErrFailed)
	}
	return nonEmpty(resp.Choices[0].Message.Content)
}

func nonEmpty(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: 応答が空です", ErrFailed)
	}
	return s, nil
}
