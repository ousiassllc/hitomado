package main

import (
	"context"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store"
)

// accountLookup は gmail.AccountLookup の store 実装。アカウントが使う OAuth クライアントの資格情報の参照元を返す。
type accountLookup struct{ st store.Store }

func (l accountLookup) OAuthCredentials(ctx context.Context, accountID string) (string, string, error) {
	a, err := l.st.GetAccount(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("get account: %w", err)
	}
	c, err := l.st.GetOAuthClient(ctx, a.OAuthClientID)
	if err != nil {
		return "", "", fmt.Errorf("get oauth client: %w", err)
	}
	return c.ClientID, c.ID, nil
}
