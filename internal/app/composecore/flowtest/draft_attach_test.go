package flowtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

func reopen(t *testing.T, c *composeFixture, mode, draftID string) dto.ComposeSession {
	t.Helper()
	req := dto.OpenComposeRequest{Mode: mode, AccountID: c.A.ID, DraftID: draftID}
	if mode != "new" {
		req = dto.OpenComposeRequest{Mode: mode, Ref: &dto.ThreadRef{AccountID: c.A.ID, ThreadID: threadID}, DraftID: draftID}
	}
	s, err := c.Svc.OpenCompose(t.Context(), req)
	require.NoError(t, err)
	return s
}

func draftAttachments(c *composeFixture, draftID string) []model.AttachmentInfo {
	return c.Gmail.Drafts[apptest.Key(c.A.ID, draftID)].Attachments
}

func TestDraftAttachmentsRoundTrip(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	p := filepath.Join(t.TempDir(), "note.txt")
	require.NoError(t, os.WriteFile(p, []byte("12345"), 0o600))
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{p})
	require.NoError(t, err)
	c := toContent("件名", "<p>本文</p>", "dave@x.example")

	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	require.Len(t, draftAttachments(f, saved.DraftID), 1)
	assert.Equal(t, "note.txt", draftAttachments(f, saved.DraftID)[0].Filename)

	// 開き直すと添付が復元される（ローカルのファイルが無くなっていても、下書きから取得して保持する）。
	require.NoError(t, os.Remove(p))
	r := reopen(t, f, "new", saved.DraftID)
	require.Len(t, r.Attachments, 1)
	assert.Equal(t, "note.txt", r.Attachments[0].Filename)
	assert.EqualValues(t, 5, r.Attachments[0].Size)
	assert.Equal(t, "local", r.Attachments[0].Source)
	assert.Nil(t, r.AttachmentError)

	// 再保存（UpdateDraft でメッセージ ID が変わる）しても添付は保たれ、再度復元できる。
	again, err := f.Svc.SaveDraft(t.Context(), r.SessionID, r.Content)
	require.NoError(t, err)
	assert.Equal(t, saved.DraftID, again.DraftID)
	assert.Equal(t, 1, f.Gmail.CallCount("UpdateDraft"))
	require.Len(t, draftAttachments(f, saved.DraftID), 1)
	r2 := reopen(t, f, "new", saved.DraftID)
	require.Len(t, r2.Attachments, 1)

	// 復元したセッションからそのまま送信できる（保持したバイト列で MIME を組む）。
	_, err = f.PrepareAndSend(t, r.SessionID, c, false)
	require.NoError(t, err)
	env, _ := f.LastSent(t)
	require.Len(t, env.Attachments, 1)
	assert.Equal(t, "note.txt", env.Attachments[0].FileName)
	assert.Equal(t, []byte("12345"), env.Attachments[0].Content)
	assert.NotContains(t, f.Gmail.Drafts, apptest.Key(f.A.ID, saved.DraftID))
}

func TestRemovingRestoredAttachmentRemovesItFromDraft(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "a.bin", 10), writeFile(t, "b.bin", 20)})
	require.NoError(t, err)
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "b"))
	require.NoError(t, err)

	r := reopen(t, f, "new", saved.DraftID)
	require.Len(t, r.Attachments, 2)
	left, err := f.Svc.RemoveAttachment(t.Context(), r.SessionID, r.Attachments[0].ID)
	require.NoError(t, err)
	require.Len(t, left, 1)
	_, err = f.Svc.SaveDraft(t.Context(), r.SessionID, r.Content)
	require.NoError(t, err)
	require.Len(t, draftAttachments(f, saved.DraftID), 1)
	assert.Equal(t, "b.bin", draftAttachments(f, saved.DraftID)[0].Filename)

	_, err = f.Svc.RemoveAttachment(t.Context(), r.SessionID, left[0].ID)
	require.NoError(t, err)
	_, err = f.Svc.SaveDraft(t.Context(), r.SessionID, r.Content)
	require.NoError(t, err)
	assert.Empty(t, draftAttachments(f, saved.DraftID), "全部削除して再保存すると下書きから消える")
}

func putSourceAttachments(t *testing.T, c *composeFixture) {
	t.Helper()
	m := c.Gmail.Threads[apptest.Key(c.A.ID, threadID)].Messages[0]
	m.Attachments = []model.AttachmentInfo{
		{ID: "att1", Filename: "a.txt", MimeType: "text/plain", Size: 5},
		{ID: "att2", Filename: "b.txt", MimeType: "text/plain", Size: 5},
	}
	c.PutThread(t, c.A.ID, threadID, m)
	c.Gmail.Attachments[apptest.Key(c.A.ID, "m1", "att1")] = []byte("hello")
	c.Gmail.Attachments[apptest.Key(c.A.ID, "m1", "att2")] = []byte("world")
}

func TestForwardedAttachmentsSurviveDraftRoundTrip(t *testing.T) {
	f := newComposeFixture(t)
	putSourceAttachments(t, f)
	s := f.Open(t, "forward")
	require.Len(t, s.Attachments, 2)
	// b.txt は削除してから保存する。
	_, err := f.Svc.RemoveAttachment(t.Context(), s.SessionID, s.Attachments[1].ID)
	require.NoError(t, err)

	c := s.Content
	c.To = []model.Address{addr("dave@x.example")}
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	_, err = f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	assert.Equal(t, 1, f.Gmail.CallCount("GetAttachment"), "転送元添付は同一セッション内で再取得しない")
	require.Len(t, draftAttachments(f, saved.DraftID), 1)

	// 転送元メッセージの添付が取れなくなっても、下書き経由の復元・送信ができる。
	delete(f.Gmail.Attachments, apptest.Key(f.A.ID, "m1", "att1"))
	delete(f.Gmail.Attachments, apptest.Key(f.A.ID, "m1", "att2"))
	r := reopen(t, f, "forward", saved.DraftID)
	require.Len(t, r.Attachments, 1, "削除した転送元添付は復活しない")
	assert.Equal(t, "a.txt", r.Attachments[0].Filename)
	assert.Equal(t, "local", r.Attachments[0].Source)
}

func TestForwardDraftSendsRestoredAttachment(t *testing.T) {
	f := newComposeFixture(t)
	putSourceAttachments(t, f)
	s := f.Open(t, "forward")
	c := s.Content
	c.To = []model.Address{addr("dave@x.example")}
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	require.NoError(t, f.Svc.CloseCompose(t.Context(), s.SessionID))
	delete(f.Gmail.Attachments, apptest.Key(f.A.ID, "m1", "att1"))
	delete(f.Gmail.Attachments, apptest.Key(f.A.ID, "m1", "att2"))

	r := reopen(t, f, "forward", saved.DraftID)
	require.Len(t, r.Attachments, 2)
	_, err = f.PrepareAndSend(t, r.SessionID, r.Content, false)
	require.NoError(t, err)
	env, _ := f.LastSent(t)
	require.Len(t, env.Attachments, 2)
	assert.Equal(t, []byte("hello"), env.Attachments[0].Content)
	assert.Equal(t, []byte("world"), env.Attachments[1].Content)
}

func TestRestoreAttachmentsOverLimitKeepsBody(t *testing.T) {
	f := newComposeFixture(t)
	f.Gmail.Drafts[apptest.Key(f.A.ID, "d1")] = gmail.DraftFull{ID: "d1", Message: gmail.MessageFull{
		MessageMeta: gmail.MessageMeta{ID: "dm1", To: []model.Address{addr("zed@x.example")}, Subject: "大きい下書き"},
		BodyHTML:    "<p>draft body</p>",
	}, Attachments: []model.AttachmentInfo{
		{ID: "a1", Filename: "big1.bin", MimeType: "application/octet-stream", Size: 20 << 20},
		{ID: "a2", Filename: "big2.bin", MimeType: "application/octet-stream", Size: 6 << 20},
	}}
	r := reopen(t, f, "new", "d1")
	assert.Equal(t, "<p>draft body</p>", r.Content.BodyHTML)
	assert.Equal(t, "大きい下書き", r.Content.Subject)
	assert.Empty(t, r.Attachments)
	require.NotNil(t, r.AttachmentError)
	assert.Equal(t, "ATTACHMENT_TOO_LARGE", r.AttachmentError["code"])
	assert.Equal(t, 0, f.Gmail.CallCount("GetAttachment"), "上限超過なら添付を取得しない")
}

func TestRestoreAttachmentFetchFailureFailsOpen(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "a.bin", 10)})
	require.NoError(t, err)
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "b"))
	require.NoError(t, err)

	f.Gmail.Err["GetAttachment"] = gmail.ErrAttachmentGone
	_, err = f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID, DraftID: saved.DraftID})
	requireCode(t, "ATTACHMENT_IO", err)
}

func TestSaveDraftFailsWhenForwardedAttachmentCannotBeFetched(t *testing.T) {
	f := newComposeFixture(t)
	putSourceAttachments(t, f)
	s := f.Open(t, "forward")
	delete(f.Gmail.Attachments, apptest.Key(f.A.ID, "m1", "att1"))
	_, err := f.Svc.SaveDraft(t.Context(), s.SessionID, s.Content)
	requireCode(t, "ATTACHMENT_IO", err)
	assert.Empty(t, f.Gmail.Drafts, "添付を取得できないまま添付なしの下書きを作らない")
}

func TestRestoredAttachmentsAreInConfirmationHash(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "a.bin", 10)})
	require.NoError(t, err)
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "<p>b</p>", "dave@x.example"))
	require.NoError(t, err)

	r := reopen(t, f, "new", saved.DraftID)
	prep, err := f.Svc.PrepareSend(t.Context(), r.SessionID, r.Content, dto.SendOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, prep.AttachmentCount)
	_, err = f.Svc.RemoveAttachment(t.Context(), r.SessionID, r.Attachments[0].ID)
	require.NoError(t, err)
	_, err = f.Svc.Send(t.Context(), r.SessionID, prep.ConfirmToken, r.Content)
	requireCode(t, "CONFIRM_MISMATCH", err)
}

func TestRestoredAttachmentsCountTowardLimit(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "a.bin", 20<<20)})
	require.NoError(t, err)
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "b"))
	require.NoError(t, err)

	r := reopen(t, f, "new", saved.DraftID)
	_, err = f.Svc.AddAttachments(t.Context(), r.SessionID, []string{writeFile(t, "b.bin", 6<<20)})
	requireCode(t, "ATTACHMENT_TOO_LARGE", err)
}
