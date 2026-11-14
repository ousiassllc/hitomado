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
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system"`
	Messages     []chatMessage `json:"messages"`
	OutputConfig *effortConfig `json:"output_config,omitempty"` // エフォートがあるときだけ（#111）
}

// effortConfig は Anthropic の output_config と OpenAI Responses の reasoning が持つ effort。
type effortConfig struct {
	Effort string `json:"effort"`
}

// effortConfigOf は effort が空なら nil（要求に載せない）。
func effortConfigOf(effort string) *effortConfig {
	if effort == "" {
		return nil
	}
	return &effortConfig{Effort: effort}
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// GenerateReply は Provider の実装。
func (a *anthropic) GenerateReply(ctx context.Context, in ReplyInput) (string, error) {
	return a.complete(ctx, systemPrompt(in), []chatMessage{{Role: RoleUser, Content: userPrompt(in)}}, in.Effort)
}

// SummarizeThread は Provider の実装。
func (a *anthropic) SummarizeThread(ctx context.Context, in ThreadInput) (string, error) {
	return a.complete(ctx, summarizeSystemPrompt(in), summarizeMessages(in), in.Effort)
}

// AskAboutThread は Provider の実装。
func (a *anthropic) AskAboutThread(ctx context.Context, in ThreadQuestionInput) (string, error) {
	return a.complete(ctx, askSystemPrompt(in.ThreadInput), askMessages(in), in.Effort)
}

// JudgeReply は Provider の実装。
func (a *anthropic) JudgeReply(ctx context.Context, in ReplyJudgeInput) (ReplyJudgment, error) {
	return judgeWith(in, func(system string, msgs []chatMessage) (string, error) {
		return a.complete(ctx, system, msgs, in.Effort)
	})
}

// complete は system 指示とメッセージ列を Messages API へ送り、応答のテキストを返す。
func (a *anthropic) complete(ctx context.Context, system string, messages []chatMessage, effort string) (string, error) {
	req := anthropicRequest{
		Model:        a.model,
		MaxTokens:    anthropicMaxTokens,
		System:       system,
		Messages:     messages,
		OutputConfig: effortConfigOf(effort),
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
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"` // エフォートがあるときだけ（#111）
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
	return o.complete(ctx, systemPrompt(in), []chatMessage{{Role: RoleUser, Content: userPrompt(in)}}, in.Effort)
}

// SummarizeThread は Provider の実装。
func (o *openAI) SummarizeThread(ctx context.Context, in ThreadInput) (string, error) {
	return o.complete(ctx, summarizeSystemPrompt(in), summarizeMessages(in), in.Effort)
}

// AskAboutThread は Provider の実装。
func (o *openAI) AskAboutThread(ctx context.Context, in ThreadQuestionInput) (string, error) {
	return o.complete(ctx, askSystemPrompt(in.ThreadInput), askMessages(in), in.Effort)
}

// JudgeReply は Provider の実装。
func (o *openAI) JudgeReply(ctx context.Context, in ReplyJudgeInput) (ReplyJudgment, error) {
	return judgeWith(in, func(system string, msgs []chatMessage) (string, error) {
		return o.complete(ctx, system, msgs, in.Effort)
	})
}

// complete は system 指示とメッセージ列を Chat Completions へ送り、応答のテキストを返す。
func (o *openAI) complete(ctx context.Context, system string, messages []chatMessage, effort string) (string, error) {
	req := openAIRequest{
		Model:           o.model,
		Messages:        append([]chatMessage{{Role: "system", Content: system}}, messages...),
		ReasoningEffort: effort,
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
