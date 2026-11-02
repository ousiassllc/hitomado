package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/palette"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// tokenForgetter は gmail.Client の実装が持つ任意メソッド（再接続時に古いトークンのキャッシュを捨てる）。
type tokenForgetter interface{ Forget(accountID string) }

// authorize は OAuth クライアントで認可フローを実行する。同時に 2 つは実行しない。
// CancelAddAccount と ctx キャンセルはどちらも OAUTH_CANCELLED。
func (s *AccountService) authorize(ctx context.Context, oauthClientID string) (oauth.Result, error) {
	client, err := s.d.Store.GetOAuthClient(ctx, oauthClientID)
	if err != nil {
		return oauth.Result{}, apperr.FromInfra(err, "")
	}
	secret, err := s.d.Secrets.Get(secrets.OAuthClientSecretKey(client.ID))
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return oauth.Result{}, validation("OAuth クライアントの Client Secret が登録されていません。")
		}
		return oauth.Result{}, apperr.FromInfra(err, "")
	}

	actx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.cancelAuth != nil {
		s.mu.Unlock()
		return oauth.Result{}, validation("別のアカウント認可が進行中です。")
	}
	s.cancelAuth = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelAuth = nil
		s.mu.Unlock()
	}()

	res, err := s.d.OAuth.Authorize(actx, oauth.ClientConfig{ClientID: client.ClientID, ClientSecret: secret})
	if err != nil {
		if errors.Is(err, oauth.ErrCancelled) || errors.Is(err, context.Canceled) || actx.Err() != nil {
			return oauth.Result{}, apperr.Wrap(apperr.OAuthCancelled, "アカウントの追加をキャンセルしました。", err)
		}
		return oauth.Result{}, apperr.FromInfra(err, "")
	}
	res.Email = strings.ToLower(trimmed(res.Email))
	if res.Email == "" || res.RefreshToken == "" {
		s.revokeUnsaved(ctx, res, oauthClientID)
		return oauth.Result{}, apperr.New(apperr.GmailError, "Google から必要な情報を取得できませんでした。")
	}
	return res, nil
}

// StartAddAccount は認可の完了・キャンセル・タイムアウトまでブロックする。ctx キャンセルは OAUTH_CANCELLED。
func (s *AccountService) StartAddAccount(ctx context.Context, oauthClientID string) (dto.Account, error) {
	res, err := s.authorize(ctx, oauthClientID)
	if err != nil {
		return dto.Account{}, err
	}
	ctx = context.WithoutCancel(ctx) // 認可後の保存は中断しない

	existing, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		// 既存アカウントと同じ認可を共有していないか確認できないため、失効させない（既存を巻き込まない）
		return dto.Account{}, apperr.FromInfra(err, "")
	}
	used := make([]string, 0, len(existing))
	for _, a := range existing {
		if a.Email == res.Email {
			s.revokeUnsaved(ctx, res, oauthClientID)
			return dto.Account{}, duplicateAccount()
		}
		used = append(used, a.Color)
	}
	name := trimmed(res.Name)
	if name == "" {
		name, _, _ = strings.Cut(res.Email, "@")
	}
	now := s.d.Clock.Now()
	acc := store.Account{
		ID: store.NewID(), OAuthClientID: oauthClientID, Email: res.Email, DisplayName: name,
		Color: palette.NextUnused(used), NotifyEnabled: true, Status: store.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.d.Store.CreateAccount(ctx, acc); err != nil {
		s.revokeUnsaved(ctx, res, oauthClientID)
		if errors.Is(err, store.ErrDuplicate) {
			return dto.Account{}, duplicateAccount()
		}
		return dto.Account{}, apperr.FromInfra(err, "")
	}
	if err := s.d.Secrets.Set(secrets.RefreshTokenKey(acc.ID), res.RefreshToken); err != nil {
		if derr := s.d.Store.DeleteAccount(ctx, acc.ID); derr != nil {
			logger(s.d).Warn("アカウント追加のロールバックに失敗", "accountId", acc.ID, "err", derr)
		}
		s.revokeToken(ctx, res.RefreshToken)
		return dto.Account{}, apperr.FromInfra(err, "")
	}
	s.d.Sync.Start(acc.ID)
	saved, err := s.d.Store.GetAccount(ctx, acc.ID)
	if err != nil {
		return dto.Account{}, apperr.FromInfra(err, acc.ID)
	}
	return accountDTO(saved), nil
}

func duplicateAccount() *apperr.Error {
	return apperr.New(apperr.AccountDuplicate, "このアドレスのアカウントはすでに登録されています。")
}

// CancelAddAccount は進行中の認可（追加・再接続）を中断する。進行中でなければ何もしない。
func (s *AccountService) CancelAddAccount() error {
	s.mu.Lock()
	cancel := s.cancelAuth
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Reconnect は needs_reauth のアカウントを再認可する。認可されたアドレスが accounts.email と異なれば
// VALIDATION で拒否し、トークンは保存しない。
func (s *AccountService) Reconnect(ctx context.Context, accountID string) (dto.Account, error) {
	acc, err := s.d.Store.GetAccount(ctx, accountID)
	if err != nil {
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	res, err := s.authorize(ctx, acc.OAuthClientID)
	if err != nil {
		return dto.Account{}, err
	}
	ctx = context.WithoutCancel(ctx)
	if res.Email != acc.Email {
		s.revokeUnsaved(ctx, res, acc.OAuthClientID)
		return dto.Account{}, validation("認可されたアドレスがこのアカウントと異なります。同じアドレスで認可してください。").ForAccount(accountID)
	}
	if err := s.d.Secrets.Set(secrets.RefreshTokenKey(accountID), res.RefreshToken); err != nil {
		s.revokeToken(ctx, res.RefreshToken)
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	if err := s.d.Store.SetAccountStatus(ctx, accountID, store.StatusActive, ""); err != nil {
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	forgetAccounts(s.d, accountID)
	s.d.Sync.Start(accountID)
	s.d.Events.Emit(events.AccountStatus, events.AccountStatusPayload{AccountID: accountID, Status: store.StatusActive})
	saved, err := s.d.Store.GetAccount(ctx, accountID)
	if err != nil {
		return dto.Account{}, apperr.FromInfra(err, accountID)
	}
	return accountDTO(saved), nil
}

// revokeTimeout は保存できなかったトークンの revoke（best-effort）の待ち時間の上限。
const revokeTimeout = 10 * time.Second

// revokeToken は保存しないことにしたリフレッシュトークンを Google 側で失効させる（失敗は無視。トークンはログに出さない）。
func (s *AccountService) revokeToken(ctx context.Context, token string) {
	if token == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	defer cancel()
	if err := s.d.OAuth.Revoke(ctx, token); err != nil {
		logger(s.d).Info("保存しないトークンの revoke に失敗（無視）", "err", err)
	}
}

// revokeUnsaved は認可結果 res を保存できないとき、そのトークンを失効させる。
// 同じメールアドレスのアカウントが同じ OAuth クライアントで既に接続されているときは失効させない
// （Google 側では同一ユーザー・同一クライアントの認可が共有されるため、失効すると既存アカウントまで切れる）。
func (s *AccountService) revokeUnsaved(ctx context.Context, res oauth.Result, oauthClientID string) {
	if res.Email == "" {
		return // 誰の認可か特定できず、既存アカウントを巻き込まないとは言えない
	}
	list, err := s.d.Store.ListAccounts(context.WithoutCancel(ctx))
	if err != nil {
		return // 既存アカウントを巻き込まないことを優先する
	}
	for _, a := range list {
		if a.Email == res.Email && a.OAuthClientID == oauthClientID {
			return
		}
	}
	s.revokeToken(ctx, res.RefreshToken)
}
