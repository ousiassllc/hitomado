package aicoretests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func newDraftCtx() dto.NewDraftContext {
	return dto.NewDraftContext{Subject: "打ち合わせの日程", To: []model.Address{{Name: "Hanako", Email: "hanako@x.example"}, {Email: "taro@x.example"}}}
}

// 新規作成は、指示と入力済みの件名・宛先、送信元の表示名と署名から本文を作る。署名・引用は付けず、HTML にして返す（#365）。
func TestGenerateNewDraftBuildsTheInputFromTheInstructionAndFields(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		return "お世話になっております。\n日程の相談です。", nil
	}
	s := f.openNew(t, f.a.ID)

	got, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", "  来週で日程を相談  ", newDraftCtx())
	require.NoError(t, err)
	assert.Equal(t, "<p>お世話になっております。<br>日程の相談です。</p>", got.BodyHTML)
	assert.NotContains(t, got.BodyHTML, "Sig A")

	require.Len(t, f.provider.inputs, 1)
	in := f.provider.inputs[0]
	assert.Equal(t, "来週で日程を相談", in.Instruction)
	assert.Equal(t, "name me@a.example", in.AccountDisplayName)
	assert.Equal(t, "Sig A", in.SignatureText)
	assert.Empty(t, in.Messages, "新規作成にはスレッドの本文が無い")
	require.NotNil(t, in.New)
	assert.Equal(t, "打ち合わせの日程", in.New.Subject)
	assert.Equal(t, []string{"Hanako <hanako@x.example>", "taro@x.example"}, in.New.To)
	assert.Equal(t, [4]string{"openai", "gpt-test", "https://llm.example/v1", testAPIKey}, [4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})
}

// 指示が空・空白だけなら、生成を始めず（プロバイダを作らず）VALIDATION。上限（2,000 文字）超過も通信しない。
func TestGenerateNewDraftRequiresAnInstruction(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.openNew(t, f.a.ID)

	for _, blank := range []string{"", " ", "\n\t  \n"} {
		_, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", blank, newDraftCtx())
		requireCode(t, "VALIDATION", err)
	}
	_, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", strings.Repeat("あ", ai.MaxInstructionRunes+1), newDraftCtx())
	requireCode(t, "VALIDATION", err)
	assert.Empty(t, f.provider.inputs)
	assert.Empty(t, f.factory.provider, "プロバイダを作らない")

	_, err = f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r2", strings.Repeat("あ", ai.MaxInstructionRunes), newDraftCtx())
	require.NoError(t, err)
}

// 新規作成の生成は new セッションだけ。返信・全員に返信・転送・存在しないセッションは通信しない。返信の生成は、new を受けない（従来どおり）。
func TestGenerateNewDraftOnlyForNewSessions(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	_, err := f.ai.GenerateNewDraft(t.Context(), "missing", "r1", "x", newDraftCtx())
	requireCode(t, "SESSION_NOT_FOUND", err)
	for _, mode := range []string{"reply", "reply_all", "forward"} {
		_, err = f.ai.GenerateNewDraft(t.Context(), f.open(t, mode).SessionID, "r1", "x", newDraftCtx())
		requireCode(t, "VALIDATION", err)
	}
	_, err = f.ai.GenerateNewDraft(t.Context(), f.openNew(t, f.a.ID).SessionID, "", "x", newDraftCtx())
	requireCode(t, "VALIDATION", err)
	assert.Empty(t, f.provider.inputs)

	_, err = f.ai.GenerateReplyDraft(t.Context(), f.openNew(t, f.a.ID).SessionID, "r2", "x")
	requireCode(t, "VALIDATION", err)
	assert.Empty(t, f.provider.inputs)
}

// 設定・キー・同意の前提は返信と同じ。同意が無ければ、件名・宛先・指示を送らない。
func TestGenerateNewDraftPreconditions(t *testing.T) {
	f := newAIFixture(t)
	s := f.openNew(t, f.a.ID)
	gen := func() error {
		_, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", "指示", newDraftCtx())
		return err
	}
	requireCode(t, "AI_NOT_CONFIGURED", gen())
	_, err := f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "", "")
	require.NoError(t, err)
	requireCode(t, "AI_CONSENT_REQUIRED", gen())
	assert.Empty(t, f.provider.inputs)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	require.NoError(t, gen())
}

// 送信元が未選択の新規作成でも生成できる（表示名・署名なし）。宛先の空欄・長すぎる件名・多すぎる宛先は整えて渡す。
func TestGenerateNewDraftWithoutSenderAndMessyFields(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.openNew(t, "")

	many := make([]model.Address, 0, 30)
	many = append(many, model.Address{Email: "  "}, model.Address{Name: "山田\r\n太郎", Email: " yamada@x.example "})
	for range 28 {
		many = append(many, model.Address{Email: "x@x.example"})
	}
	_, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", "指示", dto.NewDraftContext{Subject: "a\r\nb" + strings.Repeat("あ", 600), To: many})
	require.NoError(t, err)

	in := f.provider.inputs[0]
	assert.Empty(t, in.AccountDisplayName)
	assert.Empty(t, in.SignatureText)
	assert.True(t, strings.HasPrefix(in.New.Subject, "a b"), "CR/LF は空白に畳む")
	assert.Len(t, []rune(in.New.Subject), 500)
	require.Len(t, in.New.To, 20)
	assert.Equal(t, "山田 太郎 <yamada@x.example>", in.New.To[0], "空のアドレスは捨て、名前の改行は畳む")
}

// 指示・件名・宛先はログにも設定にもエラー文面にも残さない。
func TestGenerateNewDraftDoesNotLogOrStoreTheContent(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.openNew(t, f.a.ID)
	const sensitive = "社外秘のプロジェクトXの件"
	c := dto.NewDraftContext{Subject: sensitive + "件名", To: []model.Address{{Email: "secret-person@x.example"}}}

	_, err := f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r1", sensitive, c)
	require.NoError(t, err)
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "", errors.New("boom") }
	_, err = f.ai.GenerateNewDraft(t.Context(), s.SessionID, "r2", sensitive, c)
	require.Error(t, err)
	for _, secret := range []string{sensitive, "secret-person@x.example"} {
		assert.NotContains(t, err.Error(), secret)
		assert.NotContains(t, f.logs.String(), secret)
	}
}
