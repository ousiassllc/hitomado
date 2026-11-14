package composecore_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// 画面を介さずに作る返信下書きは、「返信」を開いたときと同じ宛先・件名・In-Reply-To で、AI の本文を署名・引用の前に入れる。
// 作った下書きは Gmail にだけあり（送信しない）、作成セッションは残さない。
func TestCreateReplyDraftPutsBodyBeforeSignatureAndQuote(t *testing.T) {
	f := newComposeFixture(t)
	ref := dto.ThreadRef{AccountID: f.A.ID, ThreadID: threadID}

	draftID, to, err := f.Svc.CreateReplyDraft(t.Context(), ref, "了解です。<b>確認</b>します。\n\n追って連絡します。")
	require.NoError(t, err)
	require.NotEmpty(t, draftID)
	assert.Equal(t, []model.Address{{Name: "Alice", Email: "alice@x.example"}}, to)

	d, err := f.Gmail.GetDraft(t.Context(), f.A.ID, draftID)
	require.NoError(t, err)
	assert.Equal(t, threadID, d.ThreadID)
	assert.Equal(t, "Re: Hello", d.Message.Subject)
	assert.Equal(t, to, d.Message.To)
	body := d.Message.BodyHTML
	text, sig, quote := strings.Index(body, "追って連絡します"), strings.Index(body, "Sig A"), strings.Index(body, "Alice")
	require.True(t, text >= 0 && sig > text && quote > sig, "本文 → 署名 → 引用 の順: %s", body)
	assert.Contains(t, body, "&lt;b&gt;確認&lt;/b&gt;", "本文は HTML として解釈しない（エスケープ）")
	raw := string(f.Gmail.DraftRaw[apptest.Key(f.A.ID, draftID)])
	assert.Contains(t, raw, "In-Reply-To: <m1@x.example>")
	assert.Contains(t, raw, "<root@x.example>")

	assert.Zero(t, f.Gmail.CallCount("Send"), "下書きを作るだけで送信しない")
	assert.Equal(t, 1, f.Gmail.CallCount("CreateDraft"))
	assert.Zero(t, f.Svc.SessionCountForTest(), "作成セッションは閉じてある（開いたままにしない）")
}

// 本文が空・存在しないスレッドは、下書きを作らずエラー。
func TestCreateReplyDraftRejectsEmptyBodyAndUnknownThread(t *testing.T) {
	f := newComposeFixture(t)
	ref := dto.ThreadRef{AccountID: f.A.ID, ThreadID: threadID}

	_, _, err := f.Svc.CreateReplyDraft(t.Context(), ref, " \n ")
	requireCode(t, "VALIDATION", err)
	_, _, err = f.Svc.CreateReplyDraft(t.Context(), dto.ThreadRef{AccountID: f.A.ID, ThreadID: "none"}, "本文")
	require.Error(t, err)
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))
	assert.Zero(t, f.Svc.SessionCountForTest(), "失敗してもセッションを残さない")
}
