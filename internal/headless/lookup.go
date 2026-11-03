package headless

import (
	"context"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store"
)

// StoreLookup は gmail.AccountLookup の store 実装。アカウントが使う OAuth クライアントの資格情報の参照元を返す。
// アプリ本体（run.go）とヘッドレス同期が同じものを使う。
type StoreLookup struct{ Store store.Store }

// OAuthCredentials はアカウントの OAuth クライアントの Client ID と、Client Secret の保存キーに使う oauth_clients.id を返す。
func (l StoreLookup) OAuthCredentials(ctx context.Context, accountID string) (string, string, error) {
	a, err := l.Store.GetAccount(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("get account: %w", err)
	}
	c, err := l.Store.GetOAuthClient(ctx, a.OAuthClientID)
	if err != nil {
		return "", "", fmt.Errorf("get oauth client: %w", err)
	}
	return c.ClientID, c.ID, nil
}
