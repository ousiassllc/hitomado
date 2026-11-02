// Package secrets は OS の資格情報ストア（Windows 資格情報マネージャー）の抽象を提供する。
package secrets

import "errors"

// ErrNotFound はキーが存在しないことを表す。
var ErrNotFound = errors.New("secrets: not found")

// Store は資格情報ストア。値（トークン・Client Secret・API キー）をログに出さないこと。
type Store interface {
	Get(key string) (string, error) // 無ければ ErrNotFound
	Set(key, value string) error
	Delete(key string) error // 無くてもエラーにしない
}

// キー名は hitomado/<種別>/<ID>（ER §1）。

// RefreshTokenKey はアカウントのリフレッシュトークンのキー。
func RefreshTokenKey(accountID string) string { return "hitomado/refresh_token/" + accountID }

// OAuthClientSecretKey は OAuth クライアントの Client Secret のキー。
func OAuthClientSecretKey(oauthClientID string) string {
	return "hitomado/oauth_client_secret/" + oauthClientID
}

// AIAPIKeyKey は AI プロバイダ（anthropic／openai）の API キーのキー。
func AIAPIKeyKey(provider string) string { return "hitomado/ai_api_key/" + provider }

// MCPTokenKey は内蔵 MCP サーバーの Bearer トークンのキー。
func MCPTokenKey() string { return "hitomado/mcp_token" }
