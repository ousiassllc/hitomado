package aicoretests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
)

func TestGenerateProviderErrors(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")
	cases := map[error]string{
		ai.ErrUnauthorized:                         "AI_FAILED",
		errors.Join(ai.ErrFailed, errors.New("x")): "AI_FAILED",
	}
	for perr, want := range cases {
		f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "", perr }
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
		requireCode(t, want, err)
	}
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "  ", nil }
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	requireCode(t, "AI_FAILED", err)
}

func TestGenerateTimeoutIsAIFailed(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.ai = aicore.New((*deps.Deps)(f.Deps), f.svc, aicore.WithTimeout(10*time.Millisecond))
	f.provider.fn = func(ctx context.Context, _ ai.ReplyInput) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	s := f.open(t, "reply")
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	ae := requireError(t, "AI_FAILED", err)
	assert.True(t, ae.Retryable)
}

func TestCancelGenerate(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.entered = make(chan struct{})
	f.provider.fn = func(ctx context.Context, _ ai.ReplyInput) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	s := f.open(t, "reply")
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r-cancel", "")
		done <- err
	}()
	<-f.provider.entered
	require.NoError(t, f.ai.CancelGenerate("r-cancel"))
	requireCode(t, "CANCELLED", <-done)
	require.NoError(t, f.ai.CancelGenerate("unknown"))
}

func TestGenerateCallerContextCancelled(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.entered = make(chan struct{})
	f.provider.fn = func(ctx context.Context, _ ai.ReplyInput) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	s := f.open(t, "reply")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.GenerateReplyDraft(ctx, s.SessionID, "r1", "")
		done <- err
	}()
	<-f.provider.entered
	cancel()
	requireCode(t, "CANCELLED", <-done)
}

func TestGenerateDuplicateRequestID(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.entered = make(chan struct{})
	release := make(chan struct{})
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		<-release
		return "ok", nil
	}
	s := f.open(t, "reply")
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "dup", "")
		done <- err
	}()
	<-f.provider.entered
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "dup", "")
	requireCode(t, "VALIDATION", err)
	close(release)
	require.NoError(t, <-done)
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "", "")
	requireCode(t, "VALIDATION", err)
}

func TestGenerateFactoryError(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.Deps.AI = func(string, string, string, string) (ai.Provider, error) { return nil, errors.New("bad cfg") }
	s := f.open(t, "reply")
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	requireCode(t, "AI_FAILED", err)
}
