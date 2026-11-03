package app

import (
	"context"
	"slices"
	"sync"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/palette"
	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// AccountService は OAuth クライアント・アカウントの管理、署名、通知設定（バインディング仕様 §2）。
// OAuth クライアント CRUD は oauthclient.go、認可フロー（追加・再接続）は account_auth.go。
type AccountService struct {
	d *Deps

	mu         sync.Mutex
	cancelAuth context.CancelFunc // 進行中の認可（追加・再接続）。同時に 1 つだけ
}

// NewAccountService は AccountService を作る。
func NewAccountService(d *Deps) *AccountService { return &AccountService{d: d} }

// ListAccounts は sortOrder 昇順のアカウント一覧を返す。
func (s *AccountService) ListAccounts(ctx context.Context) ([]dto.Account, error) {
	list, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	return accountsDTO(list), nil
}

// RemoveAccount はアカウントのデータ（DB・資格情報）を削除し、可能なら Google 側のトークンも失効させる。
// 先に同期を止めて実行中の周期の終了を待ってから DB を削除する（DB 削除に失敗したら同期を再開）。revoke の失敗は無視する。
func (s *AccountService) RemoveAccount(ctx context.Context, accountID string) error {
	acc, err := s.d.Store.GetAccount(ctx, accountID)
	if err != nil {
		return apperr.FromInfra(err, accountID)
	}
	token, _ := s.d.Secrets.Get(secrets.RefreshTokenKey(accountID)) // 無ければ revoke を省くだけ

	s.d.Sync.Stop(accountID)
	if err := s.d.Store.DeleteAccount(ctx, accountID); err != nil {
		if acc.Status != store.StatusNeedsReauth { // 再接続待ちのアカウントは元々同期していない
			s.d.Sync.Start(accountID) // 削除できなかったので同期を元に戻す
		}
		return apperr.FromInfra(err, accountID)
	}
	if err := s.d.Secrets.Delete(secrets.RefreshTokenKey(accountID)); err != nil {
		logger(s.d).Warn("リフレッシュトークンの削除に失敗", "accountId", accountID, "err", err)
	}
	if token != "" {
		if err := s.d.OAuth.Revoke(ctx, token); err != nil {
			logger(s.d).Info("トークンの revoke に失敗（無視）", "accountId", accountID, "err", err)
		}
	}
	forgetAccounts(s.d, accountID)
	if s.d.Threads != nil {
		s.d.Threads.InvalidateAccount(accountID)
	}
	return nil
}

// UpdateAccount は表示名・色・通知設定を更新する。
func (s *AccountService) UpdateAccount(ctx context.Context, accountID string, patch dto.AccountPatch) (dto.Account, error) {
	p := store.AccountPatch{NotifyEnabled: patch.NotifyEnabled}
	if patch.DisplayName != nil {
		name := trimmed(*patch.DisplayName)
		if name == "" {
			return dto.Account{}, validation("表示名を入力してください。")
		}
		p.DisplayName = &name
	}
	if patch.Color != nil {
		if !palette.IsValid(*patch.Color) {
			return dto.Account{}, validation("色が正しくありません。")
		}
		p.Color = patch.Color
	}
	acc, err := s.d.Store.UpdateAccount(ctx, accountID, p)
	if err != nil {
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	return accountDTO(acc), nil
}

// SetSignature は署名を保存する。許可タグ以外はここで除去する。
func (s *AccountService) SetSignature(ctx context.Context, accountID, signatureHTML string) (dto.Account, error) {
	acc, err := s.d.Store.SetSignature(ctx, accountID, signature.Sanitize(signatureHTML))
	if err != nil {
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	return accountDTO(acc), nil
}

// ReorderAccounts は全アカウント ID を過不足なく受け取って並び順を更新する。
func (s *AccountService) ReorderAccounts(ctx context.Context, accountIDs []string) ([]dto.Account, error) {
	list, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	if len(accountIDs) != len(list) {
		return nil, validation("すべてのアカウントを過不足なく指定してください。")
	}
	seen := make(map[string]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if _, dup := seen[id]; dup || !slices.ContainsFunc(list, func(a store.Account) bool { return a.ID == id }) {
			return nil, validation("すべてのアカウントを過不足なく指定してください。")
		}
		seen[id] = struct{}{}
	}
	if err := s.d.Store.ReorderAccounts(ctx, accountIDs); err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	return s.ListAccounts(ctx)
}
