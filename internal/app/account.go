package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AccountService は OAuth クライアント・アカウントの管理、署名、通知設定（バインディング仕様 §2）。
type AccountService struct{ d *Deps }

// NewAccountService は AccountService を作る。
func NewAccountService(d *Deps) *AccountService { return &AccountService{d: d} }

func (s *AccountService) ListOAuthClients(ctx context.Context) ([]dto.OAuthClient, error) {
	return nil, errNotImplemented
}

func (s *AccountService) AddOAuthClient(ctx context.Context, name, clientID, clientSecret string) (dto.OAuthClient, error) {
	return dto.OAuthClient{}, errNotImplemented
}

// UpdateOAuthClient の clientSecret は空文字なら更新しない。
func (s *AccountService) UpdateOAuthClient(ctx context.Context, id, name, clientID, clientSecret string) (dto.OAuthClient, error) {
	return dto.OAuthClient{}, errNotImplemented
}

func (s *AccountService) DeleteOAuthClient(ctx context.Context, id string) error {
	return errNotImplemented
}

func (s *AccountService) ListAccounts(ctx context.Context) ([]dto.Account, error) {
	return nil, errNotImplemented
}

// StartAddAccount は認可の完了・キャンセル・タイムアウトまでブロックする。ctx キャンセルは OAUTH_CANCELLED。
func (s *AccountService) StartAddAccount(ctx context.Context, oauthClientID string) (dto.Account, error) {
	return dto.Account{}, errNotImplemented
}

func (s *AccountService) CancelAddAccount() error { return errNotImplemented }

func (s *AccountService) Reconnect(ctx context.Context, accountID string) (dto.Account, error) {
	return dto.Account{}, errNotImplemented
}

func (s *AccountService) RemoveAccount(ctx context.Context, accountID string) error {
	return errNotImplemented
}

func (s *AccountService) UpdateAccount(ctx context.Context, accountID string, patch dto.AccountPatch) (dto.Account, error) {
	return dto.Account{}, errNotImplemented
}

func (s *AccountService) SetSignature(ctx context.Context, accountID, signatureHTML string) (dto.Account, error) {
	return dto.Account{}, errNotImplemented
}

func (s *AccountService) ReorderAccounts(ctx context.Context, accountIDs []string) ([]dto.Account, error) {
	return nil, errNotImplemented
}
