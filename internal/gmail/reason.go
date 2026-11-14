package gmail

import "errors"

// 再接続が必要（ErrNeedsReauth／ErrClientRejected）になった理由の固定の語彙。ログ（account needs reauth の reason）に使う。
// 値はトークン・メールアドレス・応答本文を含まない固定の識別子だけ。判定は ReauthReason。
const (
	ReasonInvalidGrant      = "invalid_grant"      // トークン更新が invalid_grant（失効・取り消し・「テスト」の 7 日・トークン数の上限・パスワード変更など）
	ReasonClientRejected    = "client_rejected"    // トークン更新が invalid_client／unauthorized_client／401（Client ID／Secret の拒否）
	ReasonAPIUnauthorized   = "api_401"            // Gmail API が 401（アクセストークンを取り直しても 401）
	ReasonScopeInsufficient = "scope_insufficient" // Gmail API が 403（ACCESS_TOKEN_SCOPE_INSUFFICIENT／insufficientPermissions）
	ReasonSecretNotFound    = "secret_not_found"   // 資格情報ストアにリフレッシュトークンが無い
	ReasonSecretKeyLost     = "secret_key_lost"    // 資格情報ストアの暗号鍵が失われ、保存済みの値を読めない（Android）
)

// reasonError は sentinel（ErrNeedsReauth など）のエラーに、再接続が必要になった理由を添える。
// errors.Is／errors.As は元のエラーへそのまま通る。
type reasonError struct {
	reason string
	err    error
}

func (e *reasonError) Error() string { return e.err.Error() }
func (e *reasonError) Unwrap() error { return e.err }

// WithReason は err に理由 reason（Reason*）を添える。reason が空なら err のまま。
// Client の実装（本物・テスト用のフェイク）が、再接続が必要な失敗を返すときに使う。
func WithReason(err error, reason string) error {
	if reason == "" {
		return err
	}
	return &reasonError{reason: reason, err: err}
}

// ReauthReason は、Gmail クライアントが返した再接続が必要なエラー（ErrNeedsReauth／ErrClientRejected）の理由（Reason*）を返す。
// 理由が分からないエラー（再接続が必要でないものを含む）は空文字列。
func ReauthReason(err error) string {
	var re *reasonError
	if errors.As(err, &re) {
		return re.reason
	}
	return ""
}
