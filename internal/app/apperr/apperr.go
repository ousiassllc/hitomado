// Package apperr はインフラ層のエラーをバインディング仕様 §1.2 の AppError（エラーコード）へ写像する。
// メッセージは利用者に表示してよい日本語で、機密（トークン・本文・件名・宛先）を含めない。
package apperr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/store"
)

// エラーコード（バインディング仕様 §1.2）。
const (
	Validation          = "VALIDATION"
	NotFound            = "NOT_FOUND"
	SessionNotFound     = "SESSION_NOT_FOUND"
	Offline             = "OFFLINE"
	RateLimited         = "RATE_LIMITED"
	GmailError          = "GMAIL_ERROR"
	NeedsReauth         = "NEEDS_REAUTH"
	AccountDuplicate    = "ACCOUNT_DUPLICATE"
	OAuthClientInUse    = "OAUTH_CLIENT_IN_USE"
	OAuthCancelled      = "OAUTH_CANCELLED"
	OAuthDenied         = "OAUTH_DENIED"
	OAuthTimeout        = "OAUTH_TIMEOUT"
	SenderRequired      = "SENDER_REQUIRED"
	SenderLocked        = "SENDER_LOCKED"
	SenderMismatch      = "SENDER_MISMATCH"
	NoRecipients        = "NO_RECIPIENTS"
	InvalidAddress      = "INVALID_ADDRESS"
	AttachmentTooLarge  = "ATTACHMENT_TOO_LARGE"
	AttachmentIO        = "ATTACHMENT_IO"
	ConfirmRequired     = "CONFIRM_REQUIRED"
	ConfirmExpired      = "CONFIRM_EXPIRED"
	ConfirmMismatch     = "CONFIRM_MISMATCH"
	SendUncertain       = "SEND_UNCERTAIN"
	OAuthClientRejected = "OAUTH_CLIENT_REJECTED"
	AINotConfigured     = "AI_NOT_CONFIGURED"
	AIConsentRequired   = "AI_CONSENT_REQUIRED"
	AIFailed            = "AI_FAILED"
	Cancelled           = "CANCELLED"
	Unsupported         = "UNSUPPORTED"
	Internal            = "INTERNAL"
)

// Details の reason（AppError の補足）。画面が専用の案内を出すもの。
const (
	DetailReason            = "reason"
	ReasonUsageLimit        = "usage_limit_exceeded" // AI_FAILED: ChatGPT プランの利用枠の超過（subscription_sharing_usage_limit_exceeded）
	ReasonGmailAPIDisabled  = "gmail_api_disabled"   // GMAIL_ERROR: Google Cloud のプロジェクトで Gmail API が無効（SERVICE_DISABLED／accessNotConfigured）
	ReasonScopeInsufficient = "scope_insufficient"   // NEEDS_REAUTH: 権限（スコープ）不足（ACCESS_TOKEN_SCOPE_INSUFFICIENT／insufficientPermissions）
)

// Error は AppError。JSON は {code, message, retryable, accountId?, details?}。
//
// Details には機密に当たり得る値（宛先アドレス＝OTHER_OWN_ACCOUNT の emails、添付ファイル名、件名など）が入ることがある。
// UI へ返すためのものなので、Details・Message をログに出さないこと（ログには Code と accountId だけを記録する）。
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	AccountID string         `json:"accountId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	cause     error
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Unwrap は元のエラー（ログ用。メッセージには出さない）を返す。
func (e *Error) Unwrap() error { return e.cause }

// New は code に対応する AppError を作る。retryable は code から決まる既定値（RATE_LIMITED／OFFLINE／OAUTH_TIMEOUT は true）。
func New(code, message string) *Error {
	return &Error{Code: code, Message: message, Retryable: code == RateLimited || code == Offline || code == OAuthTimeout}
}

// DuplicateAccount は同じアドレスのアカウントがすでにある（ACCOUNT_DUPLICATE）ときのエラー。
func DuplicateAccount() *Error {
	return New(AccountDuplicate, "このアドレスのアカウントはすでに登録されています。")
}

// Wrap は cause を保持した AppError を作る。
func Wrap(code, message string, cause error) *Error {
	e := New(code, message)
	e.cause = cause
	return e
}

// ForAccount は accountId を設定したコピーを返す。
func (e *Error) ForAccount(accountID string) *Error {
	c := *e
	c.AccountID = accountID
	return &c
}

// WithDetail は details にキーを追加したコピーを返す。値に機密（宛先・添付名・件名など）を入れる場合は、
// ログに出さない前提で使うこと（Error() は Code と Message のみで Details を含まないが、Marshal／ToMap は含む）。
func (e *Error) WithDetail(key string, value any) *Error {
	c := *e
	c.Details = make(map[string]any, len(e.Details)+1)
	for k, v := range e.Details {
		c.Details[k] = v
	}
	c.Details[key] = value
	return &c
}

// FromInfra はインフラ層のエラーを AppError に写像する。*Error はそのまま返す。
// 写像できないエラーは INTERNAL とし、元のエラーは Unwrap で保持する（メッセージには出さない）。
func FromInfra(err error, accountID string) *Error {
	if err == nil {
		return nil
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	e := classify(err)
	if accountID != "" {
		e = e.ForAccount(accountID)
	}
	return e
}

func classify(err error) *Error {
	var netErr net.Error
	switch {
	case errors.Is(err, gmail.ErrSendUncertain):
		// ErrOffline などより先に見る。OFFLINE のままだと再送へ誘導し、二重送信になり得る。
		return Wrap(SendUncertain, "送信済みの可能性があります。送信済みを確認してから再送してください。", err)
	case errors.Is(err, gmail.ErrScopeInsufficient):
		// ErrNeedsReauth にも分類されるので先に見る。再接続して権限を許可し直すと直る。
		return Wrap(NeedsReauth, "Gmail の権限（スコープ）が足りません。再接続して、すべての権限を許可してください。", err).
			WithDetail(DetailReason, ReasonScopeInsufficient)
	case errors.Is(err, gmail.ErrNeedsReauth):
		return Wrap(NeedsReauth, "アカウントの接続が切れました。再接続してください。", err)
	case errors.Is(err, gmail.ErrAPIDisabled):
		// 利用者が自分の Google Cloud プロジェクトで直せる。再試行では直らない。
		return Wrap(GmailError, "Google Cloud のプロジェクトで Gmail API が有効になっていません。Google Cloud コンソール（https://console.cloud.google.com/apis/library/gmail.googleapis.com）で有効にしてから、やり直してください。", err).
			WithDetail(DetailReason, ReasonGmailAPIDisabled)
	case errors.Is(err, gmail.ErrClientRejected), errors.Is(err, oauth.ErrClientRejected):
		return Wrap(OAuthClientRejected, "OAuth クライアントの設定（Client ID／Secret）を確認してください。", err)
	case errors.Is(err, oauth.ErrCancelled), errors.Is(err, context.Canceled):
		// url.Error はキャンセルでも net.Error を満たすため、ネットワーク判定より先に見る。
		return Wrap(Cancelled, "キャンセルしました。", err)
	case errors.Is(err, gmail.ErrOffline), errors.As(err, &netErr):
		return Wrap(Offline, "ネットワークに接続できません。", err)
	case errors.Is(err, gmail.ErrRateLimited):
		return Wrap(RateLimited, "Gmail の利用制限に達しました。しばらくしてからやり直してください。", err)
	case errors.Is(err, gmail.ErrNotFound), errors.Is(err, store.ErrNotFound):
		return Wrap(NotFound, "対象が見つかりません。", err)
	case errors.Is(err, gmail.ErrAttachmentGone):
		return Wrap(AttachmentIO, "添付ファイルを取得できません。", err)
	case errors.Is(err, store.ErrOAuthClientUse):
		return Wrap(OAuthClientInUse, "このOAuthクライアントを使っているアカウントがあるため削除できません。", err)
	case errors.Is(err, oauth.ErrDenied):
		return Wrap(OAuthDenied, "Google の認可が拒否されました。", err)
	case errors.Is(err, oauth.ErrTimeout):
		return Wrap(OAuthTimeout, "認可が時間内に完了しませんでした。", err)
	case errors.Is(err, ai.ErrUnauthorized):
		return Wrap(AIFailed, "AI プロバイダの認証に失敗しました。API キーを確認してください。", err)
	case errors.Is(err, ai.ErrUsageLimit):
		// details.reason で、画面が「Manage usage」を主なアクションにした表示へ切り替える（公式の UI/UX ガイドライン）。
		return Wrap(AIFailed, "ChatGPT プランの利用枠に達しました。ChatGPT の設定の「使用状況」（https://chatgpt.com/settings/usage）で確認してください。", err).
			WithDetail(DetailReason, ReasonUsageLimit)
	case errors.Is(err, ai.ErrNotEligible):
		return Wrap(AIFailed, "この ChatGPT アカウント（プラン・ワークスペース）では、ChatGPT プランを使えません。", err)
	case errors.Is(err, ai.ErrRateLimited):
		return Wrap(RateLimited, "AI プロバイダの利用制限に達しました。しばらくしてからやり直してください。", err)
	case errors.Is(err, context.DeadlineExceeded):
		return Wrap(Offline, "応答がありませんでした。", err)
	case errors.Is(err, ai.ErrFailed):
		return Wrap(AIFailed, "AI の生成に失敗しました。", err)
	case errors.Is(err, gmail.ErrGeneric):
		return Wrap(GmailError, gmailErrorMessage(err), err)
	default:
		return Wrap(Internal, "内部エラーが発生しました。", err)
	}
}

// gmailErrorMessage は ErrGeneric の利用者向けメッセージ。Gmail API の応答を受け取っていれば、原因の切り分けに使えるよう
// HTTP ステータスと Google の reason（文字種を制限済み）を添える。Google が返した message 本体は載せない。
func gmailErrorMessage(err error) string {
	const base = "Gmail でエラーが発生しました。"
	ae, ok := gmail.AsAPIError(err)
	if !ok {
		return base
	}
	if len(ae.Reasons) == 0 {
		return fmt.Sprintf("%s（HTTP %d）", base, ae.Status)
	}
	return fmt.Sprintf("%s（HTTP %d、理由: %s）", base, ae.Status, strings.Join(ae.Reasons, "、"))
}

// Marshal は Wails の MarshalError 用（UI 返却専用。details を含むためログに出さないこと）。AppError の JSON を返す（フロントでは error.cause として読める）。
func Marshal(err error) []byte {
	b, jerr := json.Marshal(FromInfra(err, ""))
	if jerr != nil {
		return nil
	}
	return b
}

// ToMap は dto の Failures／ArchiveError など map[string]any で持つ箇所向けの変換。
// details を含むため、結果をログに出さないこと（UI 返却専用）。
func ToMap(err error) map[string]any {
	e := FromInfra(err, "")
	if e == nil {
		return nil
	}
	m := map[string]any{"code": e.Code, "message": e.Message, "retryable": e.Retryable}
	if e.AccountID != "" {
		m["accountId"] = e.AccountID
	}
	if len(e.Details) > 0 {
		m["details"] = e.Details
	}
	return m
}
