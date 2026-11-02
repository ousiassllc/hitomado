package app

import (
	"context"
	"errors"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

func (s *AccountService) clientDTO(c store.OAuthClient) (dto.OAuthClient, error) {
	_, err := s.d.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return dto.OAuthClient{}, apperr.FromInfra(err, "")
	}
	return dto.OAuthClient{ID: c.ID, Name: c.Name, ClientID: c.ClientID, AccountCount: c.AccountCount, HasSecret: err == nil}, nil
}

// ListOAuthClients は登録済みクライアントを返す（Secret は返さない）。
func (s *AccountService) ListOAuthClients(ctx context.Context) ([]dto.OAuthClient, error) {
	list, err := s.d.Store.ListOAuthClients(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	out := make([]dto.OAuthClient, 0, len(list))
	for _, c := range list {
		v, err := s.clientDTO(c)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// AddOAuthClient は OAuth クライアントを登録し、Client Secret を資格情報ストアへ保存する。
func (s *AccountService) AddOAuthClient(ctx context.Context, name, clientID, clientSecret string) (dto.OAuthClient, error) {
	name, clientID = trimmed(name), trimmed(clientID)
	if name == "" || clientID == "" || trimmed(clientSecret) == "" {
		return dto.OAuthClient{}, validation("名前・Client ID・Client Secret をすべて入力してください。")
	}
	now := s.d.Clock.Now()
	c := store.OAuthClient{ID: store.NewID(), Name: name, ClientID: clientID, CreatedAt: now, UpdatedAt: now}
	if err := s.d.Store.CreateOAuthClient(ctx, c); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return dto.OAuthClient{}, validation("この Client ID はすでに登録されています。")
		}
		return dto.OAuthClient{}, apperr.FromInfra(err, "")
	}
	if err := s.d.Secrets.Set(secrets.OAuthClientSecretKey(c.ID), trimmed(clientSecret)); err != nil {
		if derr := s.d.Store.DeleteOAuthClient(context.WithoutCancel(ctx), c.ID); derr != nil {
			logger(s.d).Warn("OAuth クライアント登録のロールバックに失敗", "oauthClientId", c.ID, "err", derr)
		}
		return dto.OAuthClient{}, apperr.FromInfra(err, "")
	}
	return s.clientDTO(c)
}

// UpdateOAuthClient の clientSecret は空文字なら更新しない。
func (s *AccountService) UpdateOAuthClient(ctx context.Context, id, name, clientID, clientSecret string) (dto.OAuthClient, error) {
	name, clientID = trimmed(name), trimmed(clientID)
	if name == "" || clientID == "" {
		return dto.OAuthClient{}, validation("名前と Client ID を入力してください。")
	}
	// Client ID と Secret は組で扱う。Secret を先に保存し、DB 更新が失敗したら旧 Secret へ戻す。
	secretKey := secrets.OAuthClientSecretKey(id)
	newSecret := trimmed(clientSecret)
	var prev savedSecret
	if newSecret != "" {
		var err error
		if prev, err = s.saveSecret(secretKey); err != nil {
			return dto.OAuthClient{}, err
		}
		if err := s.d.Secrets.Set(secretKey, newSecret); err != nil {
			return dto.OAuthClient{}, apperr.FromInfra(err, "")
		}
	}
	if _, err := s.d.Store.UpdateOAuthClient(ctx, id, name, clientID); err != nil {
		if newSecret != "" {
			s.restoreSecret(secretKey, prev)
		}
		if errors.Is(err, store.ErrDuplicate) {
			return dto.OAuthClient{}, validation("この Client ID はすでに登録されています。")
		}
		return dto.OAuthClient{}, apperr.FromInfra(err, "")
	}
	// Client ID／Secret が変わると、このクライアントを使うアカウントの認証情報（キャッシュ済みトークン源）は古くなる。
	if err := s.forgetClientAccounts(ctx, id); err != nil {
		return dto.OAuthClient{}, err
	}
	// AccountCount を含む最新の行を一覧から引く。
	list, err := s.d.Store.ListOAuthClients(ctx)
	if err != nil {
		return dto.OAuthClient{}, apperr.FromInfra(err, "")
	}
	for _, c := range list {
		if c.ID == id {
			return s.clientDTO(c)
		}
	}
	return dto.OAuthClient{}, apperr.New(apperr.NotFound, "OAuth クライアントが見つかりません。")
}

// DeleteOAuthClient は使用中でなければクライアントと Client Secret を削除する。
// Secret の削除に失敗したときは DB 行を残してエラーを返し、DB の削除に失敗したときは Secret を元に戻す。
func (s *AccountService) DeleteOAuthClient(ctx context.Context, id string) error {
	// 削除できるのは使用中のアカウントが 0 件のときだけなので、認証情報キャッシュを破棄する対象はない。
	secretKey := secrets.OAuthClientSecretKey(id)
	prev, err := s.saveSecret(secretKey)
	if err != nil {
		return err
	}
	if err := s.d.Secrets.Delete(secretKey); err != nil {
		return apperr.FromInfra(err, "")
	}
	if err := s.d.Store.DeleteOAuthClient(ctx, id); err != nil {
		s.restoreSecret(secretKey, prev)
		return apperr.FromInfra(err, "")
	}
	return nil
}

// savedSecret は資格情報ストアの Secret の退避値。found=false は元々存在しなかったことを表す。
type savedSecret struct {
	value string
	found bool
}

func (s *AccountService) saveSecret(key string) (savedSecret, error) {
	v, err := s.d.Secrets.Get(key)
	if errors.Is(err, secrets.ErrNotFound) {
		return savedSecret{}, nil
	}
	if err != nil {
		return savedSecret{}, apperr.FromInfra(err, "")
	}
	return savedSecret{value: v, found: true}, nil
}

// restoreSecret は退避した Secret を書き戻す（元々無ければ削除する）。失敗は警告ログのみ。
func (s *AccountService) restoreSecret(key string, prev savedSecret) {
	var err error
	if prev.found {
		err = s.d.Secrets.Set(key, prev.value)
	} else {
		err = s.d.Secrets.Delete(key)
	}
	if err != nil {
		logger(s.d).Warn("Client Secret の復元に失敗", "key", key, "err", err)
	}
}

// forgetClientAccounts は OAuth クライアント id を使うすべてのアカウントの Gmail 認証情報キャッシュを破棄する。
func (s *AccountService) forgetClientAccounts(ctx context.Context, clientID string) error {
	list, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		return apperr.FromInfra(err, "")
	}
	for _, a := range list {
		if a.OAuthClientID == clientID {
			forgetAccounts(s.d, a.ID)
		}
	}
	return nil
}
