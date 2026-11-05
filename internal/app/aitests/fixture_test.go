// Package aitests は AI のモデル一覧（#76・#105）の、保存前の選択・同意との組み合わせのテスト。
// 設定・同意・生成の単体のテストは internal/app/aicore にある。
package aitests

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/deps"
)

const (
	testAPIKey   = "sk-secret-test-key"          //nolint:gosec // テスト用のダミー値
	chatgptToken = "CHATGPT-ACCESS-TOKEN-SECRET" //nolint:gosec // テスト用のダミー値
)

type aiFixture struct {
	*apptest.Env
	ai   *aicore.Service
	logs *bytes.Buffer
}

func newAIFixture(t *testing.T) *aiFixture {
	t.Helper()
	f := &aiFixture{Env: apptest.NewEnv(t), logs: &bytes.Buffer{}}
	f.Deps.Logger = slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.ai = aicore.New((*deps.Deps)(f.Deps), nil) // 一覧の取得に返信セッションは要らない
	return f
}

// ready は OpenAI 互換（readyBaseURL）の設定・キー・同意をすべて済ませる。
func (f *aiFixture) ready(t *testing.T) {
	t.Helper()
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", readyBaseURL, "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
}

// chatgptReady は ChatGPT にログインし、プロバイダ・モデルを設定して同意する。
func (f *aiFixture) chatgptReady(t *testing.T) {
	t.Helper()
	f.ChatGPT.Token = chatgptToken
	_, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "", "")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt", "")
	require.NoError(t, err)
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
}
