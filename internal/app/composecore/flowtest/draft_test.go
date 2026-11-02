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

func TestSaveDraftRequiresSender(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, "")
	_, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "b"))
	requireCode(t, "SENDER_REQUIRED", err)
	assert.Empty(t, f.Gmail.Drafts)
}

func TestSaveDraftCreatesThenUpdates(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)

	r1, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "<p>one</p>"))
	require.NoError(t, err, "宛先が空でも保存できる")
	assert.NotEmpty(t, r1.DraftID)
	assert.Equal(t, f.Clock.Now().UnixMilli(), r1.SavedAt)
	assert.Equal(t, 1, f.Gmail.CallCount("CreateDraft"))

	r2, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v2", "<p>two</p>", "dave@x.example"))
	require.NoError(t, err)
	assert.Equal(t, r1.DraftID, r2.DraftID)
	assert.Equal(t, 1, f.Gmail.CallCount("CreateDraft"))
	assert.Equal(t, 1, f.Gmail.CallCount("UpdateDraft"))
	assert.Contains(t, string(f.Gmail.DraftRaw[apptest.Key(f.A.ID, r1.DraftID)]), "v2")
}

func TestSaveDraftRecreatesWhenDeletedRemotely(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	r1, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	require.NoError(t, err)
	delete(f.Gmail.Drafts, apptest.Key(f.A.ID, r1.DraftID))
	delete(f.Gmail.DraftRaw, apptest.Key(f.A.ID, r1.DraftID))
	r2, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v2", "b"))
	require.NoError(t, err)
	assert.NotEqual(t, r1.DraftID, r2.DraftID)
}

func TestReplyDraftIsBoundToThread(t *testing.T) {
	f := newComposeFixture(t)
	s := f.Open(t, "reply")
	r, err := f.Svc.SaveDraft(t.Context(), s.SessionID, contentOf(s))
	require.NoError(t, err)
	assert.Equal(t, threadID, f.Gmail.Drafts[apptest.Key(f.A.ID, r.DraftID)].ThreadID)
	assert.Contains(t, string(f.Gmail.DraftRaw[apptest.Key(f.A.ID, r.DraftID)]), "In-Reply-To: <m1@x.example>")
}

func TestOpenComposeRestoresDraft(t *testing.T) {
	f := newComposeFixture(t)
	f.Gmail.Drafts[apptest.Key(f.A.ID, "d1")] = gmail.DraftFull{ID: "d1", ThreadID: threadID, Message: gmail.MessageFull{
		MessageMeta: gmail.MessageMeta{To: []model.Address{addr("zed@x.example")}, Subject: "Re: Hello (draft)"},
		BodyHTML:    "<p>draft body</p>",
	}, Bcc: []model.Address{addr("hidden@x.example")}}
	s, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{
		Mode: "reply", Ref: &dto.ThreadRef{AccountID: f.A.ID, ThreadID: threadID}, DraftID: "d1",
	})
	require.NoError(t, err)
	assert.Equal(t, "d1", s.DraftID)
	assert.Equal(t, "<p>draft body</p>", s.Content.BodyHTML)
	assert.Equal(t, "Re: Hello (draft)", s.Content.Subject)
	assert.Equal(t, "zed@x.example", s.Content.To[0].Email)
	require.Len(t, s.Content.Bcc, 1)
	assert.Equal(t, "hidden@x.example", s.Content.Bcc[0].Email)
	assert.Empty(t, s.Attachments)

	r, err := f.Svc.SaveDraft(t.Context(), s.SessionID, contentOf(s))
	require.NoError(t, err)
	assert.Equal(t, "d1", r.DraftID, "復元した下書きを更新する")

	_, err = f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", DraftID: "d1"})
	requireCode(t, "VALIDATION", err)
	_, err = f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID, DraftID: "nope"})
	requireCode(t, "NOT_FOUND", err)
}

func TestDiscardDeletesDraftAndSession(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	r, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	require.NoError(t, err)
	require.NoError(t, f.Svc.DiscardCompose(t.Context(), s.SessionID))
	assert.NotContains(t, f.Gmail.Drafts, apptest.Key(f.A.ID, r.DraftID))
	_, err = f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	requireCode(t, "SESSION_NOT_FOUND", err)
	require.NoError(t, f.Svc.DiscardCompose(t.Context(), s.SessionID), "破棄済みの再破棄は何もしない")
}

func TestDiscardKeepsSessionWhenDraftDeleteFails(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	require.NoError(t, err)
	f.Gmail.Err["DeleteDraft"] = gmail.ErrOffline
	requireCode(t, "OFFLINE", f.Svc.DiscardCompose(t.Context(), s.SessionID))
	delete(f.Gmail.Err, "DeleteDraft")
	require.NoError(t, f.Svc.DiscardCompose(t.Context(), s.SessionID))
	assert.Empty(t, f.Gmail.Drafts)
}

func TestCloseKeepsDraft(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	r, err := f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	require.NoError(t, err)
	require.NoError(t, f.Svc.CloseCompose(t.Context(), s.SessionID))
	assert.Contains(t, f.Gmail.Drafts, apptest.Key(f.A.ID, r.DraftID))
	_, err = f.Svc.SaveDraft(t.Context(), s.SessionID, toContent("v1", "b"))
	requireCode(t, "SESSION_NOT_FOUND", err)
}

func writeFile(t *testing.T, name string, size int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, make([]byte, size), 0o600))
	return p
}

func TestAddAttachmentsLimit(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	const mb = 1024 * 1024

	got, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "a.bin", 20*mb)})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "local", got[0].Source)
	assert.Equal(t, "a.bin", got[0].Filename)
	assert.EqualValues(t, 20*mb, got[0].Size)

	small, big := writeFile(t, "b.bin", 1*mb), writeFile(t, "c.bin", 5*mb+1)
	_, err = f.Svc.AddAttachments(t.Context(), s.SessionID, []string{small, big})
	ae := requireError(t, "ATTACHMENT_TOO_LARGE", err)
	assert.EqualValues(t, 25*mb, ae.Details["limitBytes"])

	// 追加分はすべて不採用（small も入らない）。ちょうど 25MB は許可。
	got, err = f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "d.bin", 5*mb)})
	require.NoError(t, err)
	assert.Len(t, got, 2)

	got, err = f.Svc.RemoveAttachment(t.Context(), s.SessionID, got[0].ID)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestAddAttachmentsUnreadable(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{filepath.Join(t.TempDir(), "missing.txt")})
	requireCode(t, "ATTACHMENT_IO", err)
	_, err = f.Svc.AddAttachments(t.Context(), s.SessionID, []string{t.TempDir()})
	requireCode(t, "ATTACHMENT_IO", err)
}

func TestLocalAttachmentReadAtSendTime(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.NewMail(t)
	p := filepath.Join(t.TempDir(), "note.txt")
	require.NoError(t, os.WriteFile(p, []byte("12345"), 0o600))
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{p})
	require.NoError(t, err)

	prep, err := f.Svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, prep.AttachmentCount)
	_, err = f.Svc.Send(t.Context(), s.SessionID, prep.ConfirmToken, c)
	require.NoError(t, err)
	env, _ := f.LastSent(t)
	require.Len(t, env.Attachments, 1)
	assert.Equal(t, []byte("12345"), env.Attachments[0].Content)
}

func TestLocalAttachmentMissingAtSend(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.NewMail(t)
	p := filepath.Join(t.TempDir(), "note.txt")
	require.NoError(t, os.WriteFile(p, []byte("12345"), 0o600))
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{p})
	require.NoError(t, err)
	prep, err := f.Svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	require.NoError(t, os.Remove(p))
	_, err = f.Svc.Send(t.Context(), s.SessionID, prep.ConfirmToken, c)
	requireCode(t, "ATTACHMENT_IO", err)
	assert.Empty(t, f.Gmail.Sent)
}

func TestAttachmentChangeInvalidatesConfirmation(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.NewMail(t)
	prep, err := f.Svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	_, err = f.Svc.AddAttachments(t.Context(), s.SessionID, []string{writeFile(t, "x.bin", 10)})
	require.NoError(t, err)
	_, err = f.Svc.Send(t.Context(), s.SessionID, prep.ConfirmToken, c)
	requireCode(t, "CONFIRM_MISMATCH", err)
}
