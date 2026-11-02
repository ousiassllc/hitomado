package composecore_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

func TestDraftRoundTripKeepsBcc(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	c := toContent("件名", "<p>本文</p>", "to@x.example")
	c.Bcc = []model.Address{addr("hidden@x.example")}
	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)

	got, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID, DraftID: saved.DraftID})
	require.NoError(t, err)
	require.Len(t, got.Content.Bcc, 1)
	assert.Equal(t, "hidden@x.example", got.Content.Bcc[0].Email)
	assert.Equal(t, "to@x.example", got.Content.To[0].Email)

	_, err = f.Svc.SaveDraft(t.Context(), got.SessionID, got.Content)
	require.NoError(t, err)
	assert.Contains(t, string(f.Gmail.DraftRaw[apptest.Key(f.A.ID, saved.DraftID)]), "hidden@x.example", "再保存で Bcc が保たれる")
}

func TestBccOnlyRecipients(t *testing.T) {
	f := newComposeFixture(t)
	s := f.OpenNew(t, f.A.ID)
	c := toContent("件名", "<p>本文</p>")
	c.Bcc = []model.Address{addr("hidden@x.example")}

	_, err := f.Svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	_, err = f.PrepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err, "Bcc のみでも NO_RECIPIENTS にならない")
	env, _ := f.LastSent(t)
	assert.Contains(t, string(f.Gmail.Sent[0].Raw), "hidden@x.example")
	assert.Empty(t, env.GetHeader("To"))
}

// draftGate は CreateDraft を呼び出し側が解放するまで止める gmail.Client。
type draftGate struct {
	gmail.Client
	entered chan struct{}
	release chan struct{}
}

func (g *draftGate) CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (string, error) {
	close(g.entered)
	select {
	case <-g.release:
	case <-ctx.Done():
	}
	return g.Client.CreateDraft(ctx, accountID, raw, threadID)
}

func TestSendDeletesDraftCreatedWhileWaiting(t *testing.T) {
	f := newComposeFixture(t)
	g := &draftGate{Client: f.Gmail, entered: make(chan struct{}), release: make(chan struct{})}
	f.Deps.Gmail = g
	svc := composecore.New((*deps.Deps)(f.Deps))
	s, err := svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID})
	require.NoError(t, err)
	c := toContent("s", "<p>b</p>", "dave@x.example")
	p, err := svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)

	saveDone, sendDone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := svc.SaveDraft(t.Context(), s.SessionID, c)
		saveDone <- err
	}()
	<-g.entered // SaveDraft は io ロックを持ったまま CreateDraft で止まっている
	go func() {
		_, err := svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
		sendDone <- err
	}()
	require.Eventually(t, func() bool { return svc.SendingForTest(s.SessionID) }, 5*time.Second, time.Millisecond)
	close(g.release)

	require.NoError(t, <-saveDone)
	require.NoError(t, <-sendDone)
	assert.Len(t, f.Gmail.Sent, 1)
	assert.Empty(t, f.Gmail.Drafts, "待機中に作られた下書きも送信後に削除される")
}

func TestLocalAttachmentModTimeChangeBlocksSend(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.NewMail(t)
	p := filepath.Join(t.TempDir(), "secret-name.txt")
	require.NoError(t, os.WriteFile(p, []byte("12345"), 0o600))
	_, err := f.Svc.AddAttachments(t.Context(), s.SessionID, []string{p})
	require.NoError(t, err)
	prep, err := f.Svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)

	// 同じサイズのまま内容と更新時刻が変わる。
	require.NoError(t, os.WriteFile(p, []byte("54321"), 0o600))
	require.NoError(t, os.Chtimes(p, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))
	_, err = f.Svc.Send(t.Context(), s.SessionID, prep.ConfirmToken, c)
	ae := requireError(t, "ATTACHMENT_IO", err)
	assert.Equal(t, "secret-name.txt", ae.Details["filename"])
	assert.NotContains(t, ae.Message, "secret-name", "メッセージにファイル名を含めない")
	assert.Empty(t, f.Gmail.Sent)
}

func TestForwardedAttachmentSizeMismatchBlocksSend(t *testing.T) {
	f := newComposeFixture(t)
	m := f.Gmail.Threads[apptest.Key(f.A.ID, threadID)].Messages[0]
	m.Attachments = []model.AttachmentInfo{{ID: "att1", Filename: "a.txt", MimeType: "text/plain", Size: 5}}
	f.PutThread(t, f.A.ID, threadID, m)
	f.Gmail.Attachments[apptest.Key(f.A.ID, aliceMsg, "att1")] = []byte("hello!")
	s := f.Open(t, "forward")
	c := contentOf(s)
	c.To = []model.Address{addr("dave@x.example")}
	_, err := f.PrepareAndSend(t, s.SessionID, c, false)
	requireCode(t, "ATTACHMENT_IO", err)
	assert.Empty(t, f.Gmail.Sent)
}

func TestOpenComposeFoldsLineBreaksFromHeaders(t *testing.T) {
	f := newComposeFixture(t)
	m := f.Gmail.Threads[apptest.Key(f.A.ID, threadID)].Messages[0]
	m.Subject = "Hello\r\nBcc: evil@x.example"
	m.From = model.Address{Name: "Al\r\nice", Email: "alice@x.example"}
	m.Cc = []model.Address{{Name: "Ca\nrol", Email: "carol@x.example"}}
	f.PutThread(t, f.A.ID, threadID, m)

	s := f.Open(t, "reply_all")
	assert.Equal(t, "Re: Hello Bcc: evil@x.example", s.Content.Subject)
	assert.Equal(t, "Al ice", s.Content.To[0].Name)
	assert.Equal(t, "Ca rol", s.Content.Cc[len(s.Content.Cc)-1].Name)
	for _, a := range append(append([]model.Address{}, s.Content.To...), s.Content.Cc...) {
		assert.NotContains(t, a.Name+a.Email, "\n")
		assert.NotContains(t, a.Name+a.Email, "\r")
	}
	_, err := f.Svc.SaveDraft(t.Context(), s.SessionID, s.Content)
	require.NoError(t, err)
}

func TestSendStripsInternalMarkersButDraftKeepsThem(t *testing.T) {
	f := newComposeFixture(t)
	s := f.Open(t, "reply")
	body := s.Content.BodyHTML
	require.Contains(t, body, "data-hitomado-signature")
	require.Contains(t, body, "data-hitomado-quote")
	require.Contains(t, body, "data-account-id")

	saved, err := f.Svc.SaveDraft(t.Context(), s.SessionID, s.Content)
	require.NoError(t, err)
	draft := string(f.Gmail.DraftRaw[apptest.Key(f.A.ID, saved.DraftID)])
	assert.True(t, strings.Contains(draft, "data-hitomado-signature") || strings.Contains(draft, "data-hitomado-signature=3D"),
		"下書きには署名・引用の検出用マーカーを残す")

	_, err = f.PrepareAndSend(t, s.SessionID, s.Content, false)
	require.NoError(t, err)
	env, _ := f.LastSent(t)
	assert.NotContains(t, env.HTML, "data-hitomado")
	assert.NotContains(t, env.HTML, "data-account-id")
	assert.Contains(t, env.HTML, "Sig A", "ブロック自体は残る")
	assert.Contains(t, env.HTML, "<blockquote>")
}

// deleteGate は DeleteDraft を呼び出し側が解放するまで止める gmail.Client。
type deleteGate struct {
	gmail.Client
	entered chan struct{}
	release chan struct{}
}

func (g *deleteGate) DeleteDraft(ctx context.Context, accountID, draftID string) error {
	close(g.entered)
	select {
	case <-g.release:
	case <-ctx.Done():
	}
	return g.Client.DeleteDraft(ctx, accountID, draftID)
}

func TestSendAbortsWhenSessionDiscardedWhileWaiting(t *testing.T) {
	f := newComposeFixture(t)
	svc := composecore.New((*deps.Deps)(f.Deps))
	s, err := svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID})
	require.NoError(t, err)
	c := toContent("s", "<p>b</p>", "dave@x.example")
	_, err = svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	p, err := svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)

	g := &deleteGate{Client: f.Gmail, entered: make(chan struct{}), release: make(chan struct{})}
	f.Deps.Gmail = g
	discardDone, sendDone := make(chan error, 1), make(chan error, 1)
	go func() { discardDone <- svc.DiscardCompose(t.Context(), s.SessionID) }()
	<-g.entered // Discard は snapshot を通り、io ロックを持ったまま DeleteDraft で止まっている
	go func() {
		_, err := svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
		sendDone <- err
	}()
	require.Eventually(t, func() bool { return svc.SendingForTest(s.SessionID) }, 5*time.Second, time.Millisecond)
	close(g.release)

	require.NoError(t, <-discardDone)
	requireCode(t, "SESSION_NOT_FOUND", <-sendDone)
	assert.Empty(t, f.Gmail.Sent, "破棄済みのセッションからは送信しない")
}

func TestReplyHeadersFoldLineBreaks(t *testing.T) {
	f := newComposeFixture(t)
	m := f.Gmail.Threads[apptest.Key(f.A.ID, threadID)].Messages[0]
	m.MessageID = "<m1@x.example>\r\nBcc: evil@x.example"
	m.References = []string{"<root@x.example>\nX-Evil: 1"}
	f.PutThread(t, f.A.ID, threadID, m)
	s := f.Open(t, "reply")
	_, err := f.PrepareAndSend(t, s.SessionID, contentOf(s), false)
	require.NoError(t, err)
	raw := string(f.Gmail.Sent[0].Raw)
	assert.NotContains(t, raw, "\nBcc: evil")
	assert.NotContains(t, raw, "\nX-Evil")
}
