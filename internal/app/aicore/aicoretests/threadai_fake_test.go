package aicoretests

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// SummarizeThread は入力を記録し、entered を閉じ、threadFn（無ければ固定の要約）で応答する。
func (p *fakeProvider) SummarizeThread(ctx context.Context, in ai.ThreadInput) (string, error) {
	p.mu.Lock()
	p.summaries = append(p.summaries, in)
	p.mu.Unlock()
	return p.threadResult(ctx)
}

// AskAboutThread は SummarizeThread と同じく、入力を記録して応答する。
func (p *fakeProvider) AskAboutThread(ctx context.Context, in ai.ThreadQuestionInput) (string, error) {
	p.mu.Lock()
	p.questions = append(p.questions, in)
	p.mu.Unlock()
	return p.threadResult(ctx)
}

func (p *fakeProvider) threadResult(ctx context.Context) (string, error) {
	if p.entered != nil {
		close(p.entered)
	}
	if p.threadFn != nil {
		return p.threadFn(ctx)
	}
	return "- 回答", nil
}
