package siwc

import (
	"context"
	"errors"
	"slices"
	"time"
)

// SignOut は Auth の実装。失効はベストエフォート（通信失敗・5xx は再試行し、確認できなくてもローカルは必ず消す）。
// 消すのは指定したアカウントのトークンだけで、ほかのアカウントのトークン・登録と、使用中のアカウントの選択は変えない。
func (m *Manager) SignOut(ctx context.Context, accountID string) (bool, error) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return false, err
	}
	acct := reg.find(accountID)
	if acct == nil {
		return false, ErrUnknownAccount
	}
	t, err := m.loadTokens(acct.ClientID)
	if err != nil {
		return false, err
	}
	revoked := t.Refresh == "" // 失効するものが無ければ確認済み扱い
	if !revoked {
		revoked = m.revokeWithRetry(ctx, acct.ClientID, t.Refresh)
	}
	if err := m.clearTokens(acct.ClientID); err != nil {
		return revoked, err
	}
	err = m.updateRegistry(ctx, func(r *registry) error {
		if a := r.find(accountID); a != nil {
			a.ExpiresAt = 0
		}
		return nil
	})
	return revoked, err
}

// RemoveAccount は Auth の実装。ログアウト済みのアカウントの登録（client_id・本人情報）を忘れる。
func (m *Manager) RemoveAccount(ctx context.Context, accountID string) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return err
	}
	if reg.find(accountID) == nil {
		return ErrUnknownAccount
	}
	live, err := m.hasSession(accountID)
	if err != nil {
		return err
	}
	if live {
		return ErrAccountInUse
	}
	if err := m.clearTokens(accountID); err != nil { // 断片など、セッションと言えない残りも片づける
		return err
	}
	return m.updateRegistry(ctx, func(r *registry) error {
		r.Accounts = slices.DeleteFunc(r.Accounts, func(a account) bool { return a.ClientID == accountID })
		if r.Active == accountID {
			r.Active = ""
		}
		return nil
	})
}

// SelectAccount は Auth の実装。
func (m *Manager) SelectAccount(ctx context.Context, accountID string) error {
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return err
	}
	if reg.find(accountID) == nil {
		return ErrUnknownAccount
	}
	live, err := m.hasSession(accountID)
	if err != nil {
		return err
	}
	if !live {
		return ErrNotSignedIn
	}
	return m.updateRegistry(ctx, func(r *registry) error {
		if r.find(accountID) == nil {
			return ErrUnknownAccount // 確認から保存までの間に削除された
		}
		r.Active = accountID
		return nil
	})
}

// revokeWithRetry は失効を最大 revokeAttempts 回試す。成功（空の 200 を含む 2xx）で true。
func (m *Manager) revokeWithRetry(ctx context.Context, clientID, refreshToken string) bool {
	for i := range revokeAttempts {
		err := m.revoke(ctx, clientID, refreshToken)
		if err == nil {
			return true
		}
		var se *serverError
		if errors.As(err, &se) && se.Status < 500 {
			return false // 4xx は再試行しても変わらない
		}
		if ctx.Err() != nil {
			return false
		}
		if i < revokeAttempts-1 {
			select {
			case <-time.After(m.opts.RevokeBackoff):
			case <-ctx.Done():
				return false
			}
		}
	}
	m.opts.Logger.Warn("chatgpt: remote revocation was not confirmed")
	return false
}
