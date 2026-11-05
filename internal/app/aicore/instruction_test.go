package aicore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 指示は前後の空白を除いてプロバイダへ渡る。省略・空・空白だけは指示なし（#109）。
func TestGenerateReplyDraftPassesTheInstruction(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")

	for i, tc := range []struct{ in, want string }{
		{"", ""},
		{"  \n\t ", ""},
		{"  3 行で短く\n", "3 行で短く"},
		{"日程は来週以降で、\n丁寧に", "日程は来週以降で、\n丁寧に"},
	} {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r"+string(rune('a'+i)), tc.in)
		require.NoError(t, err)
		assert.Equal(t, tc.want, f.provider.inputs[i].Instruction)
	}
}

// 2,000 文字（rune 数）までは通り、超えると通信せず VALIDATION。エラー文面に指示を含めない。
func TestGenerateReplyDraftInstructionLimit(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")

	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", strings.Repeat("あ", ai.MaxInstructionRunes))
	require.NoError(t, err)
	require.Len(t, f.provider.inputs, 1)

	long := strings.Repeat("秘", ai.MaxInstructionRunes+1)
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2", long)
	ae := requireError(t, "VALIDATION", err)
	assert.NotContains(t, ae.Message, "秘")
	assert.Len(t, f.provider.inputs, 1, "上限超過ではプロバイダへ送らない")

	// 前後の空白は数えない。
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r3", "  "+strings.Repeat("あ", ai.MaxInstructionRunes)+"  ")
	require.NoError(t, err)
}

// 指示はログ・設定（app_settings）に残さない。プロバイダの失敗のエラー文面にも含めない。
func TestInstructionIsNotLoggedOrStored(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")
	const secret = "社外秘のプロジェクトXの件を断る"

	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", secret)
	require.NoError(t, err)
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "", errors.New("boom") }
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2", secret)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, f.logs.String(), secret)

	cfg, ok, err := f.Store.GetSetting(t.Context(), store.SettingAIConfig)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, cfg, secret)
}
