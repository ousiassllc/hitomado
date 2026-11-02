package composecore_test

import (
	"errors"
	"fmt"
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

func (f *composeFixture) newMail(t *testing.T) (dto.ComposeSession, dto.ComposeContent) {
	t.Helper()
	s := f.openNew(t, f.a.ID)
	return s, toContent("件名", "<p>本文</p>", "dave@x.example")
}

func TestPrepareSendValidationOrder(t *testing.T) {
	f := newComposeFixture(t)
	opts := dto.SendOptions{}

	s := f.openNew(t, "")
	_, err := f.svc.PrepareSend(t.Context(), s.SessionID, toContent("s", "b", "x@y.example"), opts)
	requireCode(t, "SENDER_REQUIRED", err)

	s, _ = f.newMail(t)
	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, toContent("s", "b"), opts)
	requireCode(t, "NO_RECIPIENTS", err)

	c := toContent("s", "b", "ok@y.example", "bad address", "")
	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, c, opts)
	ae := requireError(t, "INVALID_ADDRESS", err)
	assert.Equal(t, []string{"bad address"}, ae.Details["emails"])

	_, err = f.svc.PrepareSend(t.Context(), "missing", toContent("s", "b", "x@y.example"), opts)
	requireCode(t, "SESSION_NOT_FOUND", err)
}

func TestPrepareSendPreviewAndWarnings(t *testing.T) {
	f := newComposeFixture(t)
	s := f.openNew(t, f.a.ID)

	c := toContent("", s.Content.BodyHTML, "me@a.example", "me@b.example", "ME@B.example")
	p, err := f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	assert.Equal(t, f.a.ID, p.From.AccountID)
	assert.Equal(t, "me@a.example", p.From.Email)
	assert.Equal(t, f.a.Color, p.From.Color)
	assert.NotEmpty(t, p.ConfirmToken)
	assert.Equal(t, f.Clock.Now().Add(5*time.Minute).UnixMilli(), p.ExpiresAt)

	byCode := map[string]dto.SendWarning{}
	for _, w := range p.Warnings {
		byCode[w.Code] = w
	}
	require.Contains(t, byCode, "OTHER_OWN_ACCOUNT")
	assert.Equal(t, []string{"me@b.example"}, byCode["OTHER_OWN_ACCOUNT"].Emails, "自分自身宛は警告せず、他アカウントは重複なし")
	assert.Contains(t, byCode, "EMPTY_SUBJECT")
	assert.Contains(t, byCode, "EMPTY_BODY", "署名のみは本文空")

	c = toContent("件名", "<p>こんにちは</p>", "dave@x.example")
	p, err = f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	assert.Empty(t, p.Warnings)
	assert.Equal(t, "こんにちは", p.BodyPreview)
	assert.NotNil(t, p.Bcc)
}

func TestConfirmTokenEnforcement(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.newMail(t)
	send := func(tok string, content dto.ComposeContent) error {
		_, err := f.svc.Send(t.Context(), s.SessionID, tok, content)
		return err
	}

	requireCode(t, "CONFIRM_REQUIRED", send("", c))
	requireCode(t, "CONFIRM_REQUIRED", send("unknown", c))

	p1, err := f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	requireCode(t, "CONFIRM_REQUIRED", send(p1.ConfirmToken+"x", c))

	changed := c
	changed.Subject = "別の件名"
	requireCode(t, "CONFIRM_MISMATCH", send(p1.ConfirmToken, changed))
	changed = c
	changed.To = []model.Address{addr("eve@x.example")}
	requireCode(t, "CONFIRM_MISMATCH", send(p1.ConfirmToken, changed))

	// archive の有無も内容ハッシュに含まれる。
	p2, err := f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{ArchiveAfterSend: true})
	require.NoError(t, err)
	requireCode(t, "CONFIRM_REQUIRED", send(p1.ConfirmToken, c)) // 再 PrepareSend で旧トークンは無効

	f.Clock.Advance(5*time.Minute + time.Second)
	requireCode(t, "CONFIRM_EXPIRED", send(p2.ConfirmToken, c))
	assert.Empty(t, f.Gmail.Sent)
}

func TestConfirmTokenSingleUseEvenOnFailure(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.newMail(t)
	p, err := f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)

	f.Gmail.Err["Send"] = gmail.ErrOffline
	_, err = f.svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
	requireCode(t, "OFFLINE", err)

	delete(f.Gmail.Err, "Send")
	_, err = f.svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
	requireCode(t, "CONFIRM_EXPIRED", err)
	assert.Empty(t, f.Gmail.Sent)

	// セッションは残っているので、再度確認すれば送れる。
	_, err = f.prepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err)
	assert.Len(t, f.Gmail.Sent, 1)
}

func TestSendClosesSession(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.newMail(t)
	res, err := f.prepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err)
	assert.NotEmpty(t, res.MessageID)
	assert.False(t, res.Archived)
	assert.Empty(t, f.Gmail.Modified)

	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	requireCode(t, "SESSION_NOT_FOUND", err)
}

func TestSendAndArchive(t *testing.T) {
	f := newComposeFixture(t)

	s := f.open(t, "reply")
	res, err := f.prepareAndSend(t, s.SessionID, contentOf(s), true)
	require.NoError(t, err)
	assert.True(t, res.Archived)
	require.Len(t, f.Gmail.Modified, 1)
	assert.Equal(t, []string{"INBOX"}, f.Gmail.Modified[0].Remove)
	assert.Equal(t, threadID, f.Gmail.Modified[0].ThreadID)
	assert.Equal(t, f.a.ID, f.Gmail.Modified[0].AccountID)
	assert.Contains(t, f.Sync.RefreshThreadCalls, apptest.ThreadKey{AccountID: f.a.ID, ThreadID: threadID})
}

func TestSendAndArchiveFailureKeepsSendSuccessful(t *testing.T) {
	f := newComposeFixture(t)
	s := f.open(t, "reply")
	f.Gmail.Err["ModifyThread"] = gmail.ErrRateLimited

	res, err := f.prepareAndSend(t, s.SessionID, contentOf(s), true)
	require.NoError(t, err)
	assert.False(t, res.Archived)
	assert.Equal(t, "RATE_LIMITED", res.ArchiveError["code"])
	assert.Len(t, f.Gmail.Sent, 1)
}

func TestSendFailureDoesNotArchive(t *testing.T) {
	f := newComposeFixture(t)
	s := f.open(t, "reply")
	f.Gmail.Err["Send"] = errors.New("boom")
	_, err := f.prepareAndSend(t, s.SessionID, contentOf(s), true)
	requireCode(t, "INTERNAL", err)
	assert.Empty(t, f.Gmail.Modified)
}

func TestSendDeletesDraftAndAddsContacts(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.newMail(t)
	c.Cc = []model.Address{addr("cc@x.example")}
	saved, err := f.svc.SaveDraft(t.Context(), s.SessionID, c)
	require.NoError(t, err)
	require.Contains(t, f.Gmail.Drafts, apptest.Key(f.a.ID, saved.DraftID))

	_, err = f.prepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err)
	assert.NotContains(t, f.Gmail.Drafts, apptest.Key(f.a.ID, saved.DraftID))

	got, err := f.svc.SuggestContacts(t.Context(), f.a.ID, "dave", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "dave@x.example", got[0].Email)
	got, err = f.svc.SuggestContacts(t.Context(), f.a.ID, "CC@", 10)
	require.NoError(t, err)
	assert.Len(t, got, 1, "prefix は正規化される")

	other, err := f.svc.SuggestContacts(t.Context(), f.b.ID, "dave", 10)
	require.NoError(t, err)
	assert.Empty(t, other, "アカウントごとに分離")
	empty, err := f.svc.SuggestContacts(t.Context(), f.a.ID, "  ", 10)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestSuggestContactsLimitClamped(t *testing.T) {
	f := newComposeFixture(t)
	s := f.openNew(t, f.a.ID)
	c := toContent("s", "b")
	for i := range 25 {
		c.To = append(c.To, addr(fmt.Sprintf("u%02d@x.example", i)))
	}
	_, err := f.prepareAndSend(t, s.SessionID, c, false)
	require.NoError(t, err)
	got, err := f.svc.SuggestContacts(t.Context(), f.a.ID, "", 100)
	require.NoError(t, err)
	assert.Empty(t, got)
	got, err = f.svc.SuggestContacts(t.Context(), f.a.ID, "u", 100)
	require.NoError(t, err)
	assert.Len(t, got, 20)
	got, err = f.svc.SuggestContacts(t.Context(), f.a.ID, "u", 0)
	require.NoError(t, err)
	assert.Len(t, got, 1, "limit は 1 以上に丸める")
}

func TestConcurrentSendIsExclusive(t *testing.T) {
	f := newComposeFixture(t)
	bg := &blockingGmail{Client: f.Gmail, entered: make(chan struct{}), release: make(chan struct{})}
	f.Deps.Gmail = bg
	svc := composecore.New((*deps.Deps)(f.Deps))
	s, err := svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.a.ID})
	require.NoError(t, err)
	c := toContent("s", "<p>b</p>", "dave@x.example")
	p, err := svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
		done <- err
	}()
	<-bg.entered

	_, err = svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
	requireCode(t, "VALIDATION", err)
	_, err = svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	requireCode(t, "VALIDATION", err)

	close(bg.release)
	require.NoError(t, <-done)
	assert.Len(t, f.Gmail.Sent, 1, "送信は 1 回だけ")
}

func TestSessionExpiresAfter24Hours(t *testing.T) {
	f := newComposeFixture(t)
	s, c := f.newMail(t)
	f.Clock.Advance(23 * time.Hour)
	_, err := f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err, "操作すると最終操作時刻が更新される")
	f.Clock.Advance(23 * time.Hour)
	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	require.NoError(t, err)
	f.Clock.Advance(24*time.Hour + time.Second)
	_, err = f.svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
	requireCode(t, "SESSION_NOT_FOUND", err)
}
