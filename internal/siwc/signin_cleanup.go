package siwc

import (
	"context"
	"time"
)

// restoreTokens はアカウントのトークンを prev（ログイン前の値。無ければ空）に戻す（そのアカウントのロックを持って呼ぶ）。
// 戻せなかったときは、新旧が混ざったトークンを残さないよう消す。戻した旧トークンが OpenAI 側でまだ有効かは分からない
// （無効なら次の更新で ErrReauthRequired になり、再ログインを案内する）。ログにトークンの値は出さない。
func (m *Manager) restoreTokens(clientID string, prev tokens) {
	if prev.Refresh != "" {
		if err := m.saveTokens(clientID, prev); err == nil {
			return
		}
		m.opts.Logger.Warn("chatgpt: could not restore the previous session tokens")
	}
	if err := m.clearTokens(clientID); err != nil {
		m.opts.Logger.Warn("chatgpt: could not clear the tokens of a login that was not saved")
	}
}

// declinePlan は ChatGPT プランの利用が許可されなかったログインの後始末をして ErrPlanNotGranted を返す（そのアカウントのロックを持って呼ぶ）。
// ログイン中のアカウント（このアカウントを含む）のセッションは変えない。ログイン中でないアカウントだけ、次のログインで
// 同じクライアントを使い同意を求め直すため、登録を残す（ほかのアカウントの登録・使用中のアカウントは変えない）。
func (m *Manager) declinePlan(ctx context.Context, clientID, subject, email string) error {
	if live, err := m.hasSession(clientID); err != nil {
		return err
	} else if live {
		return ErrPlanNotGranted
	}
	if err := m.updateRegistry(ctx, func(r *registry) error {
		a := r.upsert(clientID)
		a.Subject, a.Email, a.ConsentNeeded, a.ExpiresAt = subject, email, true, 0
		return nil
	}); err != nil {
		return err
	}
	return ErrPlanNotGranted
}

// revokeQuietly は失効を 1 度だけ試み、結果は問わない（検証に失敗した・不要になったトークンの後始末）。
func (m *Manager) revokeQuietly(ctx context.Context, clientID, refreshToken string) {
	if refreshToken == "" || clientID == "" {
		return
	}
	// ctx が中断済みでも後始末は短時間だけ試す。
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = m.revoke(rctx, clientID, refreshToken)
}
