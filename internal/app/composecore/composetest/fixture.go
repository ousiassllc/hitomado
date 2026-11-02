// Package composetest は composecore のテスト用フィクスチャ（2 アカウントと返信元スレッドを持つ環境）を提供する。
package composetest

import (
	"bytes"
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
	"github.com/ousiassllc/hitomado/internal/store"
)

// Fixture は 2 アカウントと、A のスレッド t1（alice からの 1 通）を持つ環境。
type Fixture struct {
	*apptest.Env
	Svc  *composecore.Service
	A, B store.Account
}

// ThreadID／AliceMsg は返信元スレッド・メッセージの ID。
const (
	ThreadID = "t1"
	AliceMsg = "m1"
)

// New はフィクスチャを作る。
func New(t *testing.T) *Fixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &Fixture{Env: e}
	f.A = e.AddAccount(t, "me@a.example")
	f.B = e.AddAccount(t, "me@b.example")
	_, err := e.Store.SetSignature(t.Context(), f.A.ID, "<p>Sig A</p>")
	require.NoError(t, err)
	f.A.SignatureHTML = "<p>Sig A</p>"

	m := apptest.Msg(AliceMsg, ThreadID, apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.To = []model.Address{{Email: "me@a.example"}, {Email: "bob@x.example"}}
	m.Cc = []model.Address{{Email: "carol@x.example"}}
	m.Subject = "Hello"
	m.MessageID = "<m1@x.example>"
	m.References = []string{"<root@x.example>"}
	e.PutThread(t, f.A.ID, ThreadID, m)
	f.Svc = composecore.New((*deps.Deps)(e.Deps))
	return f
}

func (f *Fixture) Open(t *testing.T, mode string) dto.ComposeSession {
	t.Helper()
	s, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: mode, Ref: &dto.ThreadRef{AccountID: f.A.ID, ThreadID: ThreadID}})
	require.NoError(t, err)
	return s
}

func (f *Fixture) OpenNew(t *testing.T, accountID string) dto.ComposeSession {
	t.Helper()
	s, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: accountID})
	require.NoError(t, err)
	return s
}

// RequireCode は err が want コードの apperr.Error であることを検証する。
func RequireCode(t *testing.T, want string, err error) {
	t.Helper()
	_ = RequireError(t, want, err)
}

// RequireError は RequireCode と同じ検証をして、*apperr.Error（details の確認用）を返す。
func RequireError(t *testing.T, want string, err error) *apperr.Error {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
	return ae
}

// Addr は名前なしのアドレス。
func Addr(email string) model.Address { return model.Address{Email: email} }

// ToContent は To だけを持つ入力内容。
func ToContent(subject, body string, to ...string) dto.ComposeContent {
	c := dto.ComposeContent{To: []model.Address{}, Cc: []model.Address{}, Bcc: []model.Address{}, Subject: subject, BodyHTML: body}
	for _, e := range to {
		c.To = append(c.To, Addr(e))
	}
	return c
}

// ContentOf はセッションの初期内容をそのまま返す入力にする（フロントが初期値を編集せず送るケース）。
// ContentOf はセッションの初期内容。
func ContentOf(s dto.ComposeSession) dto.ComposeContent { return s.Content }

// PrepareAndSend は PrepareSend→Send を続けて呼ぶ。
func (f *Fixture) PrepareAndSend(t *testing.T, id string, c dto.ComposeContent, archive bool) (dto.SendResult, error) {
	t.Helper()
	p, err := f.Svc.PrepareSend(t.Context(), id, c, dto.SendOptions{ArchiveAfterSend: archive})
	require.NoError(t, err)
	return f.Svc.Send(t.Context(), id, p.ConfirmToken, c)
}

func (f *Fixture) LastSent(t *testing.T) (*enmime.Envelope, apptest.SentRecord) {
	t.Helper()
	require.NotEmpty(t, f.Gmail.Sent)
	rec := f.Gmail.Sent[len(f.Gmail.Sent)-1]
	env, err := enmime.ReadEnvelope(bytes.NewReader(rec.Raw))
	require.NoError(t, err)
	return env, rec
}

// NewMail は送信元 A の新規作成セッションと、To に dave@x.example を持つ入力内容を返す。
func (f *Fixture) NewMail(t *testing.T) (dto.ComposeSession, dto.ComposeContent) {
	t.Helper()
	s := f.OpenNew(t, f.A.ID)
	return s, ToContent("件名", "<p>本文</p>", "dave@x.example")
}
