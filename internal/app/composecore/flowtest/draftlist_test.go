package flowtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// saveNew は account の新規作成セッションで下書きを保存し、下書き ID を返す。
func saveNew(t *testing.T, f *composeFixture, accountID, subject, body string, to ...string) string {
	t.Helper()
	s := f.OpenNew(t, accountID)
	r, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent(subject, body, to...))
	require.NoError(t, err)
	return r.DraftID
}

// 全アカウントの下書きが、アカウントを区別して、更新の新しい順の 1 つの一覧で返る。
// アプリで保存した新規・返信の下書きも、Gmail 側で作られた下書き（Drafts へ直接入れたもの）も出る。
func TestListDraftsMergesAccountsNewestFirst(t *testing.T) {
	f := newComposeFixture(t)
	oldA := saveNew(t, f, f.A.ID, "A の古い下書き", "<p>古い本文</p>", "dave@x.example")
	onB := saveNew(t, f, f.B.ID, "B の下書き", "<p>B の本文</p>")
	reply := f.Open(t, "reply")
	rr, err := f.Svc.SaveDraft(t.Context(), reply.SessionID, contentOf(reply))
	require.NoError(t, err)
	// 古い下書きを開き直して保存し直すと、更新が新しくなって先頭に来る。
	opened, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID, DraftID: oldA})
	require.NoError(t, err)
	_, err = f.Svc.SaveDraft(t.Context(), opened.SessionID, toContent("A の更新した下書き", "<p>新しい本文</p>", "dave@x.example"))
	require.NoError(t, err)

	got, err := f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, got.Failures)
	require.False(t, got.Truncated)
	require.Len(t, got.Items, 3)

	assert.Equal(t, []string{oldA, rr.DraftID, onB}, []string{got.Items[0].DraftID, got.Items[1].DraftID, got.Items[2].DraftID})
	first := got.Items[0]
	assert.Equal(t, f.A.ID, first.AccountID)
	assert.Equal(t, "A の更新した下書き", first.Subject)
	assert.Equal(t, "新しい本文", first.Snippet)
	require.Len(t, first.To, 1)
	assert.Equal(t, "dave@x.example", first.To[0].Email)
	assert.False(t, first.IsReply)
	assert.Greater(t, first.UpdatedAt, got.Items[1].UpdatedAt)

	assert.True(t, got.Items[1].IsReply, "返信の下書きは元のスレッドへ開き直せるよう、返信と分かる")
	assert.Equal(t, threadID, got.Items[1].ThreadID)
	assert.Equal(t, f.B.ID, got.Items[2].AccountID)
	assert.NotNil(t, got.Items[2].To, "宛先なしでも null にしない")
}

func TestListDraftsFiltersAccounts(t *testing.T) {
	f := newComposeFixture(t)
	saveNew(t, f, f.A.ID, "a", "<p>a</p>")
	b := saveNew(t, f, f.B.ID, "b", "<p>b</p>")

	got, err := f.Svc.ListDrafts(t.Context(), []string{f.B.ID, "unknown"})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, b, got.Items[0].DraftID)
	assert.NotContains(t, f.Gmail.Calls, "ListDrafts:"+f.A.ID, "選ばれていないアカウントは Gmail を呼ばない")

	empty, err := f.Svc.ListDrafts(t.Context(), []string{"unknown"})
	require.NoError(t, err)
	assert.NotNil(t, empty.Items)
	assert.Empty(t, empty.Items)
}

// 1 アカウントの取得失敗は Failures に入れ、ほかのアカウントの下書きは返す。
func TestListDraftsReportsFailuresPerAccount(t *testing.T) {
	f := newComposeFixture(t)
	saveNew(t, f, f.A.ID, "a", "<p>a</p>")
	b := saveNew(t, f, f.B.ID, "b", "<p>b</p>")
	f.Gmail.Err["ListDrafts:"+f.A.ID] = gmail.ErrNeedsReauth

	got, err := f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, b, got.Items[0].DraftID)
	require.Len(t, got.Failures, 1)
	assert.Equal(t, f.A.ID, got.Failures[0].AccountID)
	assert.Equal(t, "NEEDS_REAUTH", got.Failures[0].Error["code"])
}

// 一覧の「開く」は OpenCompose の draftId で、宛先・件名・本文・添付が作成画面に戻る（新規の下書き）。
func TestListedNewDraftReopensWithAttachments(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	path := filepath.Join(t.TempDir(), "memo.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{path})
	require.NoError(t, err)
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("続きを書く", "<p>途中</p>", "dave@x.example"))
	require.NoError(t, err)
	require.NoError(t, f.Svc.CloseCompose(t.Context(), s.SessionID))

	list, err := f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	d := list.Items[0]
	require.Equal(t, saved.DraftID, d.DraftID)

	re, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: d.AccountID, DraftID: d.DraftID})
	require.NoError(t, err)
	assert.Equal(t, "続きを書く", re.Content.Subject)
	assert.Equal(t, "dave@x.example", re.Content.To[0].Email)
	assert.Contains(t, re.Content.BodyHTML, "途中")
	require.Len(t, re.Attachments, 1)
	assert.Equal(t, "memo.txt", re.Attachments[0].Filename)

	// 続きを書いて送信できる（確認トークン経由）。
	res, err := f.PrepareAndSend(t, re.SessionID, toContent("続きを書く", "<p>途中の続き</p>", "dave@x.example"), false)
	require.NoError(t, err)
	assert.NotEmpty(t, res.MessageID)
}

// 返信の下書きは、元のスレッドの返信として開き直す（F-12 の復元と同じ）。
func TestListedReplyDraftReopensAsReply(t *testing.T) {
	f := newComposeFixture(t)
	reply := f.Open(t, "reply")
	c := contentOf(reply)
	c.BodyHTML = "<p>返信の下書き</p>"
	_, err := f.Svc.SaveDraft(t.Context(), reply.SessionID, c)
	require.NoError(t, err)

	list, err := f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	d := list.Items[0]
	require.True(t, d.IsReply)

	re, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{
		Mode: "reply_all", Ref: &dto.ThreadRef{AccountID: d.AccountID, ThreadID: d.ThreadID}, DraftID: d.DraftID,
	})
	require.NoError(t, err)
	assert.True(t, re.SenderLocked)
	assert.Contains(t, re.Content.BodyHTML, "返信の下書き")
}

// 破棄すると、Gmail の下書きも消えて、一覧から出なくなる。作成セッションは作らない。
func TestDiscardDraftRemovesItFromGmailAndTheList(t *testing.T) {
	f := newComposeFixture(t)
	keep := saveNew(t, f, f.A.ID, "残す", "<p>k</p>")
	drop := saveNew(t, f, f.B.ID, "捨てる", "<p>d</p>")

	list, err := f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	var ref dto.DraftRef
	for _, d := range list.Items {
		if d.DraftID == drop {
			ref = dto.DraftRef{AccountID: d.AccountID, DraftID: d.DraftID, ThreadID: d.ThreadID}
		}
	}
	require.NotEmpty(t, ref.DraftID)
	require.NoError(t, f.Svc.DiscardDraft(t.Context(), ref))

	assert.NotContains(t, f.Gmail.Drafts, apptest.Key(f.B.ID, drop))
	assert.Contains(t, f.Gmail.Drafts, apptest.Key(f.A.ID, keep))
	list, err = f.Svc.ListDrafts(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, keep, list.Items[0].DraftID)
}

// Gmail 側で既に消えている下書きの破棄は成功。削除の失敗は、下書きを残したままエラーで返す。
func TestDiscardDraftEdgeCases(t *testing.T) {
	f := newComposeFixture(t)
	require.NoError(t, f.Svc.DiscardDraft(t.Context(), dto.DraftRef{AccountID: f.A.ID, DraftID: "gone"}), "既に無い下書きは成功")
	requireCode(t, "VALIDATION", f.Svc.DiscardDraft(t.Context(), dto.DraftRef{AccountID: f.A.ID}))
	requireCode(t, "NOT_FOUND", f.Svc.DiscardDraft(t.Context(), dto.DraftRef{AccountID: "no-such-account", DraftID: "d"}))

	id := saveNew(t, f, f.A.ID, "残る", "<p>x</p>")
	f.Gmail.Err["DeleteDraft"] = gmail.ErrOffline
	requireCode(t, "OFFLINE", f.Svc.DiscardDraft(t.Context(), dto.DraftRef{AccountID: f.A.ID, DraftID: id}))
	assert.Contains(t, f.Gmail.Drafts, apptest.Key(f.A.ID, id))
}
