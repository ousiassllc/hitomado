//go:build e2e && !production

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/domain/palette"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// シードの固定値。E2E スクリプト（build/android/e2e）がこの件名・メールアドレスを画面から探す。
const (
	OAuthClientID = "e2e-oauth-client"
	AccountID     = "e2e-account"
	AccountEmail  = "e2e-user@example.com"
	SubjectPrefix = "E2E seed thread" // 件名は "E2E seed thread <n>"（n は 1 始まり）
	ThreadCount   = 3
)

func timeNow() time.Time { return time.Now().UTC() }

// Seed は DB にアカウントが無いとき OAuth クライアントとアカウントを 1 件ずつ作り、リフレッシュトークンと Client Secret を sec に置く。
// 返す FakeGmail には Profile・ラベル・スレッド（INBOX、新しいものから 1 時間ずつ古く、最新の 2 件が未読）を毎回入れる
// （FakeGmail はメモリ上のため、起動のたびに作り直す。DB 側は既にあれば作らない）。
func Seed(ctx context.Context, st store.Store, sec secrets.Store, now time.Time) (*apptest.FakeGmail, error) {
	accounts, err := st.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("e2e: list accounts: %w", err)
	}
	if len(accounts) == 0 {
		if err := createAccount(ctx, st, now); err != nil {
			return nil, err
		}
	}
	// 資格情報はメモリ上なので、DB が残っていても毎回置く。
	if err := sec.Set(secrets.OAuthClientSecretKey(OAuthClientID), "e2e-client-secret"); err != nil {
		return nil, fmt.Errorf("e2e: set client secret: %w", err)
	}
	if err := sec.Set(secrets.RefreshTokenKey(AccountID), "e2e-refresh-token"); err != nil {
		return nil, fmt.Errorf("e2e: set refresh token: %w", err)
	}
	return seedGmail(now), nil
}

func createAccount(ctx context.Context, st store.Store, now time.Time) error {
	c := store.OAuthClient{ID: OAuthClientID, Name: "E2E client", ClientID: "e2e-client-id", CreatedAt: now, UpdatedAt: now}
	if err := st.CreateOAuthClient(ctx, c); err != nil {
		return fmt.Errorf("e2e: create oauth client: %w", err)
	}
	a := store.Account{
		ID: AccountID, OAuthClientID: OAuthClientID, Email: AccountEmail, DisplayName: "E2E User",
		Color: palette.NextUnused(nil), NotifyEnabled: false, Status: store.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.CreateAccount(ctx, a); err != nil {
		return fmt.Errorf("e2e: create account: %w", err)
	}
	return nil
}

func seedGmail(now time.Time) *apptest.FakeGmail {
	f := apptest.NewFakeGmail()
	f.Profiles[AccountID] = gmail.Profile{Email: AccountEmail, HistoryID: "1"}
	f.Labels[AccountID] = []gmail.Label{
		{ID: "INBOX", Name: "INBOX", Type: "system"},
		{ID: "UNREAD", Name: "UNREAD", Type: "system"},
		{ID: "DRAFT", Name: "DRAFT", Type: "system"},
		{ID: "SENT", Name: "SENT", Type: "system"},
	}
	for n := 1; n <= ThreadCount; n++ {
		id := fmt.Sprintf("e2e-thread-%d", n)
		labels := []string{"INBOX"}
		if n <= 2 {
			labels = append(labels, "UNREAD")
		}
		m := apptest.Msg(fmt.Sprintf("e2e-msg-%d", n), id, now.Add(-time.Duration(n)*time.Hour), labels...)
		m.Subject = fmt.Sprintf("%s %d", SubjectPrefix, n)
		m.Snippet = fmt.Sprintf("Seed message %d for the Android E2E", n)
		m.To[0].Email = AccountEmail
		m.BodyHTML = fmt.Sprintf("<p>Seed message %d for the Android E2E.</p>", n)
		f.Threads[apptest.Key(AccountID, id)] = gmail.ThreadFull{ID: id, HistoryID: fmt.Sprintf("h-%d", n), Messages: []gmail.MessageFull{m}}
	}
	return f
}
