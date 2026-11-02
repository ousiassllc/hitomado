package composecore_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jhillyerd/enmime/v2"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// composeFixture は 2 アカウントと、A のスレッド t1（alice からの 1 通）を持つ環境。
type composeFixture struct {
	*apptest.Env
	svc  *composecore.Service
	a, b store.Account
}

const (
	threadID = "t1"
	aliceMsg = "m1"
)

func newComposeFixture(t *testing.T) *composeFixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &composeFixture{Env: e}
	f.a = e.AddAccount(t, "me@a.example")
	f.b = e.AddAccount(t, "me@b.example")
	_, err := e.Store.SetSignature(t.Context(), f.a.ID, "<p>Sig A</p>")
	require.NoError(t, err)
	f.a.SignatureHTML = "<p>Sig A</p>"

	m := apptest.Msg(aliceMsg, threadID, apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.To = []model.Address{{Email: "me@a.example"}, {Email: "bob@x.example"}}
	m.Cc = []model.Address{{Email: "carol@x.example"}}
	m.Subject = "Hello"
	m.MessageID = "<m1@x.example>"
	m.References = []string{"<root@x.example>"}
	e.PutThread(t, f.a.ID, threadID, m)
	f.svc = composecore.New((*deps.Deps)(e.Deps))
	return f
}

func (f *composeFixture) open(t *testing.T, mode string) dto.ComposeSession {
	t.Helper()
	s, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: mode, Ref: &dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID}})
	require.NoError(t, err)
	return s
}

func (f *composeFixture) openNew(t *testing.T, accountID string) dto.ComposeSession {
	t.Helper()
	s, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: accountID})
	require.NoError(t, err)
	return s
}

// requireCode は err が want コードの apperr.Error であることを検証する。
func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	_ = requireError(t, want, err)
}

// requireError は requireCode と同じ検証をして、*apperr.Error（details の確認用）を返す。
func requireError(t *testing.T, want string, err error) *apperr.Error {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
	return ae
}

func addr(email string) model.Address { return model.Address{Email: email} }

func toContent(subject, body string, to ...string) dto.ComposeContent {
	c := dto.ComposeContent{To: []model.Address{}, Cc: []model.Address{}, Bcc: []model.Address{}, Subject: subject, BodyHTML: body}
	for _, e := range to {
		c.To = append(c.To, addr(e))
	}
	return c
}

// contentOf はセッションの初期内容をそのまま返す入力にする（フロントが初期値を編集せず送るケース）。
func contentOf(s dto.ComposeSession) dto.ComposeContent { return s.Content }

// prepareAndSend は PrepareSend→Send を続けて呼ぶ。
func (f *composeFixture) prepareAndSend(t *testing.T, id string, c dto.ComposeContent, archive bool) (dto.SendResult, error) {
	t.Helper()
	p, err := f.svc.PrepareSend(t.Context(), id, c, dto.SendOptions{ArchiveAfterSend: archive})
	require.NoError(t, err)
	return f.svc.Send(t.Context(), id, p.ConfirmToken, c)
}

func (f *composeFixture) lastSent(t *testing.T) (*enmime.Envelope, apptest.SentRecord) {
	t.Helper()
	require.NotEmpty(t, f.Gmail.Sent)
	rec := f.Gmail.Sent[len(f.Gmail.Sent)-1]
	env, err := enmime.ReadEnvelope(bytes.NewReader(rec.Raw))
	require.NoError(t, err)
	return env, rec
}

// blockingGmail は Send を呼び出し側が解放するまで止める gmail.Client（二重送信のテスト用）。
type blockingGmail struct {
	gmail.Client
	entered chan struct{}
	release chan struct{}
}

func (b *blockingGmail) Send(ctx context.Context, accountID string, raw []byte, threadID string) (gmail.SentMessage, error) {
	close(b.entered)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return b.Client.Send(ctx, accountID, raw, threadID)
}
