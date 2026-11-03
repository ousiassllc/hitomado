package composecore_test

import (
	"net/mail"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/oauth"
)

// OAuth（Android のループバックでも同じ。リダイレクト方式は受け取る認可コードの経路だけを変える）で追加した
// アカウントの返信は、Google が返したアドレス（小文字正規化）を送信元に固定し、変更できない（Issue #86）。
func TestReplyFromAccountAddedByOAuthIsPinnedToReceivedAddress(t *testing.T) {
	f := newComposeFixture(t)
	accounts := app.NewAccountService(f.Deps)
	f.Flow.Result = oauth.Result{Email: "Mobile.User@C.Example", Name: "Mobile User", RefreshToken: "rt-mobile"}
	added, err := accounts.StartAddAccount(t.Context(), f.A.OAuthClientID)
	require.NoError(t, err)
	require.Equal(t, "mobile.user@c.example", added.Email)

	m := apptest.Msg("mm1", "tm1", apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.To = []model.Address{{Email: added.Email}}
	m.Subject = "To mobile"
	m.MessageID = "<mm1@x.example>"
	f.PutThread(t, added.ID, "tm1", m)

	s, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &dto.ThreadRef{AccountID: added.ID, ThreadID: "tm1"}})
	require.NoError(t, err)
	assert.True(t, s.SenderLocked)
	assert.Equal(t, "mobile.user@c.example", s.From.Email)

	// 別のアカウントへ変えられない。
	_, err = f.Svc.SetComposeSender(t.Context(), s.SessionID, f.B.ID)
	requireCode(t, "SENDER_LOCKED", err)

	// 防御的検査: セッションの送信元を別アカウントに差し替えられても、送信は SENDER_MISMATCH で止まる。
	p, err := f.Svc.PrepareSend(t.Context(), s.SessionID, contentOf(s), dto.SendOptions{})
	require.NoError(t, err)
	f.Svc.ForceSenderForTest(s.SessionID, f.B.ID)
	_, err = f.Svc.Send(t.Context(), s.SessionID, p.ConfirmToken, contentOf(s))
	requireCode(t, "SENDER_MISMATCH", err)
	assert.Empty(t, f.Gmail.Sent)

	// 差し替えなしなら、受け取ったアカウントから、そのアドレスを From にして送られる。
	s2, err := f.Svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &dto.ThreadRef{AccountID: added.ID, ThreadID: "tm1"}})
	require.NoError(t, err)
	c := contentOf(s2)
	c.BodyHTML = "<p>reply</p>"
	_, err = f.PrepareAndSend(t, s2.SessionID, c, false)
	require.NoError(t, err)
	env, rec := f.LastSent(t)
	assert.Equal(t, added.ID, rec.AccountID)
	from, err := mail.ParseAddress(env.GetHeader("From"))
	require.NoError(t, err)
	assert.Equal(t, "mobile.user@c.example", from.Address)
}
