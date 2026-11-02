// Package oauth はループバック＋PKCE の OAuth 認可フロー（NF-S03）を提供する。
package oauth

import (
	"context"
	"errors"
)

// Scopes は要求するスコープ（NF-S02）。
var Scopes = []string{
	"https://www.googleapis.com/auth/gmail.modify",
	"openid",
	"email",
	"profile",
}

// 認可フローのエラー。
var (
	ErrCancelled = errors.New("oauth: cancelled") // ctx のキャンセル、または利用者が認可画面を拒否せず閉じた
	ErrDenied    = errors.New("oauth: denied")    // access_denied・スコープ不足
	ErrTimeout   = errors.New("oauth: timeout")   // 5 分以内に完了しなかった

	// ErrClientRejected はトークンエンドポイントが Client ID／Secret を拒否した（invalid_client／unauthorized_client／401）。
	ErrClientRejected = errors.New("oauth: client rejected")
)

// ClientConfig は利用者が登録した OAuth クライアントの資格情報。
type ClientConfig struct {
	ClientID     string
	ClientSecret string
}

// Result は認可の結果。Email は小文字正規化済み。
type Result struct {
	Email        string
	Name         string
	RefreshToken string
}

// Flow は認可フロー。ブラウザ起動（BrowserOpener）は実装に注入する。
type Flow interface {
	// Authorize はシステムブラウザで認可画面を開き、ループバックで code を受け取って
	// トークン交換・userinfo 取得まで行う。ctx キャンセルは ErrCancelled。
	Authorize(ctx context.Context, cfg ClientConfig) (Result, error)
	// Revoke はリフレッシュトークンを Google 側で失効させる（失敗しても呼び出し側は無視してよい）。
	Revoke(ctx context.Context, refreshToken string) error
}
