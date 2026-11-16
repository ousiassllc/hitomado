package aicoretests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// 401 以外（利用枠・対象外・その他の 403 に当たる ErrFailed）ではトークンを更新せず、スレッドを再送しない。
func TestChatGPTFailureOtherThan401IsNotRetried(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	calls := 0
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		calls++
		return "", fmt.Errorf("%w: HTTP 403", ai.ErrFailed)
	}
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	requireCode(t, "AI_FAILED", err)
	assert.Equal(t, 1, calls)
	assert.Zero(t, f.ChatGPT.Refreshes)
}

// 401 のあとの更新の最中に利用者が中止したら、OAUTH_CANCELLED ではなく CANCELLED。
func TestChatGPTCancelDuringRefreshIsCancelled(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "", ai.ErrUnauthorized }
	f.ChatGPT.RefreshErr = context.Canceled
	f.ChatGPT.OnRefresh = func() { _ = f.ai.CancelGenerate("r1") }
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	requireCode(t, "CANCELLED", err)
}
