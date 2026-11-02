package aicore_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

// composeFixture は 2 アカウントと、A のスレッド t1（alice からの 1 通）を持つ環境。
type composeFixture struct {
	*apptest.Env
	svc  *composecore.Service
	a, b store.Account
}

const threadID = "t1"

func newComposeFixture(t *testing.T) *composeFixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &composeFixture{Env: e}
	f.a = e.AddAccount(t, "me@a.example")
	f.b = e.AddAccount(t, "me@b.example")
	_, err := e.Store.SetSignature(t.Context(), f.a.ID, "<p>Sig A</p>")
	require.NoError(t, err)
	f.a.SignatureHTML = "<p>Sig A</p>"

	m := apptest.Msg("m1", threadID, apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.Subject = "Hello"
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
