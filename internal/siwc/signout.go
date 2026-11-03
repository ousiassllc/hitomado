package siwc

import (
	"context"
	"errors"
	"time"
)

// SignOut は Auth の実装。失効はベストエフォート（通信失敗・5xx は再試行し、確認できなくてもローカルは必ず消す）。
func (m *Manager) SignOut(ctx context.Context) (bool, error) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	reg, err := m.loadRegistration(ctx)
	if err != nil {
		return false, err
	}
	t, err := m.loadTokens()
	if err != nil {
		return false, err
	}
	revoked := t.Refresh == "" || reg.ClientID == "" // 失効するものが無ければ確認済み扱い
	if !revoked {
		revoked = m.revokeWithRetry(ctx, reg.ClientID, t.Refresh)
	}
	if err := m.clearTokens(); err != nil {
		return revoked, err
	}
	reg.ExpiresAt = 0
	if err := m.saveRegistration(ctx, reg); err != nil {
		return revoked, err
	}
	return revoked, nil
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
