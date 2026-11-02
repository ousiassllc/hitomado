package composecore_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func TestReplyInitialValuesAndHeaders(t *testing.T) {
	f := newComposeFixture(t)
	s := f.open(t, "reply")

	assert.True(t, s.SenderLocked)
	assert.Equal(t, f.a.ID, s.AccountID)
	require.NotNil(t, s.From)
	assert.Equal(t, "me@a.example", s.From.Email)
	assert.Equal(t, []model.Address{{Name: "Alice", Email: "alice@x.example"}}, s.Content.To)
	assert.Empty(t, s.Content.Cc)
	assert.Equal(t, "Re: Hello", s.Content.Subject)
	body := s.Content.BodyHTML
	sig, quote := strings.Index(body, "data-hitomado-signature"), strings.Index(body, "data-hitomado-quote")
	require.GreaterOrEqual(t, sig, 0)
	require.GreaterOrEqual(t, quote, 0)
	assert.Less(t, sig, quote, "署名は引用より前")
	assert.Contains(t, body, "Sig A")

	_, err := f.prepareAndSend(t, s.SessionID, contentOf(s), false)
	require.NoError(t, err)
	env, rec := f.lastSent(t)
	assert.Equal(t, f.a.ID, rec.AccountID)
	assert.Equal(t, threadID, rec.ThreadID)
	assert.Equal(t, "<m1@x.example>", env.GetHeader("In-Reply-To"))
	assert.Equal(t, "<root@x.example> <m1@x.example>", env.GetHeader("References"))
	assert.Contains(t, env.GetHeader("From"), "me@a.example")
	assert.Equal(t, "Re: Hello", env.GetHeader("Subject"))
}

func TestReplyAllExcludesSelf(t *testing.T) {
	f := newComposeFixture(t)
	s := f.open(t, "reply_all")
	assert.Equal(t, []model.Address{{Name: "Alice", Email: "alice@x.example"}}, s.Content.To)
	emails := []string{}
	for _, a := range s.Content.Cc {
		emails = append(emails, a.Email)
	}
	assert.ElementsMatch(t, []string{"bob@x.example", "carol@x.example"}, emails)
}

func TestSourceSelectionSkipsOwnAndDrafts(t *testing.T) {
	f := newComposeFixture(t)
	own := apptest.Msg("m2", threadID, apptest.Epoch.Add(time.Hour), "SENT")
	own.From = model.Address{Email: "ME@a.example"}
	own.To = []model.Address{addr("alice@x.example")}
	own.Subject = "Re: Hello"
	draft := apptest.Msg("m3", threadID, apptest.Epoch.Add(2*time.Hour), "DRAFT")
	draft.From = model.Address{Email: "me@a.example"}
	first := f.Gmail.Threads[apptest.Key(f.a.ID, threadID)].Messages[0]
	f.PutThread(t, f.a.ID, threadID, first, own, draft)

	s := f.open(t, "reply")
	assert.Equal(t, "alice@x.example", s.Content.To[0].Email, "自分の送信と下書きを除く最新（alice）")

	got, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{
		Mode: "reply", Ref: &dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID}, SourceMessageID: "m2",
	})
	require.NoError(t, err)
	assert.Equal(t, "alice@x.example", got.Content.To[0].Email, "自分の送信への返信は元の To")

	f.PutThread(t, f.a.ID, threadID, own, draft)
	s = f.open(t, "reply")
	assert.Equal(t, "alice@x.example", s.Content.To[0].Email, "全部自分なら最後（自分の送信）の To")
}

func TestOpenValidation(t *testing.T) {
	f := newComposeFixture(t)
	_, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "bogus"})
	requireCode(t, "VALIDATION", err)
	_, err = f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply"})
	requireCode(t, "VALIDATION", err)
	_, err = f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &dto.ThreadRef{AccountID: f.a.ID, ThreadID: "nope"}})
	requireCode(t, "NOT_FOUND", err)
	_, err = f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: "missing"})
	requireCode(t, "NOT_FOUND", err)
}

func TestNewComposeSender(t *testing.T) {
	f := newComposeFixture(t)
	s := f.openNew(t, "")
	assert.False(t, s.SenderLocked)
	assert.Nil(t, s.From)
	assert.Empty(t, s.AccountID)

	s2 := f.openNew(t, f.a.ID)
	assert.Contains(t, s2.Content.BodyHTML, "Sig A")
	assert.Equal(t, "me@a.example", s2.From.Email)

	got, err := f.svc.SetComposeSender(t.Context(), s.SessionID, f.b.ID)
	require.NoError(t, err)
	assert.Equal(t, f.b.ID, got.AccountID)
	assert.Equal(t, "me@b.example", got.From.Email)

	_, err = f.svc.SetComposeSender(t.Context(), s.SessionID, "missing")
	requireCode(t, "NOT_FOUND", err)
}

func TestSenderLockedForRepliesAndForward(t *testing.T) {
	f := newComposeFixture(t)
	for _, mode := range []string{"reply", "reply_all", "forward"} {
		s := f.open(t, mode)
		_, err := f.svc.SetComposeSender(t.Context(), s.SessionID, f.b.ID)
		requireCode(t, "SENDER_LOCKED", err)
	}
}

func TestSenderMismatchDetectedOnPrepareAndSend(t *testing.T) {
	f := newComposeFixture(t)
	s := f.open(t, "reply")
	p, err := f.svc.PrepareSend(t.Context(), s.SessionID, contentOf(s), dto.SendOptions{})
	require.NoError(t, err)

	f.svc.ForceSenderForTest(s.SessionID, f.b.ID)
	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, contentOf(s), dto.SendOptions{})
	requireCode(t, "SENDER_MISMATCH", err)
	_, err = f.svc.Send(t.Context(), s.SessionID, p.ConfirmToken, contentOf(s))
	requireCode(t, "SENDER_MISMATCH", err)
	assert.Empty(t, f.Gmail.Sent)
}

func TestForwardAttachmentsCarriedAndRemovable(t *testing.T) {
	f := newComposeFixture(t)
	m := f.Gmail.Threads[apptest.Key(f.a.ID, threadID)].Messages[0]
	m.Attachments = []model.AttachmentInfo{
		{ID: "att1", Filename: "a.txt", MimeType: "text/plain", Size: 5},
		{ID: "att2", Filename: "b.txt", MimeType: "text/plain", Size: 5},
	}
	f.PutThread(t, f.a.ID, threadID, m)
	f.Gmail.Attachments[apptest.Key(f.a.ID, aliceMsg, "att1")] = []byte("hello")
	f.Gmail.Attachments[apptest.Key(f.a.ID, aliceMsg, "att2")] = []byte("world")

	s := f.open(t, "forward")
	require.Len(t, s.Attachments, 2)
	assert.Equal(t, "forwarded", s.Attachments[0].Source)
	assert.Empty(t, s.Content.To)
	assert.Equal(t, "Fwd: Hello", s.Content.Subject)
	assert.Contains(t, s.Content.BodyHTML, "転送されたメッセージ")

	left, err := f.svc.RemoveAttachment(t.Context(), s.SessionID, s.Attachments[1].ID)
	require.NoError(t, err)
	require.Len(t, left, 1)
	_, err = f.svc.RemoveAttachment(t.Context(), s.SessionID, "nope")
	requireCode(t, "NOT_FOUND", err)

	c := contentOf(s)
	c.To = []model.Address{addr("dave@x.example")}
	_, err = f.prepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err)
	env, rec := f.lastSent(t)
	assert.Empty(t, rec.ThreadID, "転送は新しいスレッド")
	assert.Empty(t, env.GetHeader("In-Reply-To"))
	require.Len(t, env.Attachments, 1)
	assert.Equal(t, "a.txt", env.Attachments[0].FileName)
	assert.Equal(t, []byte("hello"), env.Attachments[0].Content)
}

func TestForwardAttachmentGone(t *testing.T) {
	f := newComposeFixture(t)
	m := f.Gmail.Threads[apptest.Key(f.a.ID, threadID)].Messages[0]
	m.Attachments = []model.AttachmentInfo{{ID: "att1", Filename: "a.txt", MimeType: "text/plain", Size: 5}}
	f.PutThread(t, f.a.ID, threadID, m)
	s := f.open(t, "forward")
	c := contentOf(s)
	c.To = []model.Address{addr("dave@x.example")}
	_, err := f.prepareAndSend(t, s.SessionID, c, false)
	requireCode(t, "ATTACHMENT_IO", err)
	assert.Empty(t, f.Gmail.Sent)
}
