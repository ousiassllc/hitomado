// Package secrets は OS の資格情報ストア（Windows 資格情報マネージャー・Android の暗号化ストレージ）の抽象を提供する。
package secrets

import "errors"

// ErrNotFound はキーが存在しないことを表す。
var ErrNotFound = errors.New("secrets: not found")

// ErrKeyLost は、資格情報を暗号化している鍵（Android Keystore の鍵）が失われ、保存済みの値をすべて読めなくなった
// ことを表す（バックアップ復元・端末の移行・画面ロックのリセットなど）。ストアは作り直されて空になる。
// errors.Is(err, ErrNotFound) も真になる（値が無いのと同じ扱い）ため、既存の呼び出し側は変更なしで、
// リフレッシュトークンなら「再接続が必要」、API キー・Client Secret なら「未設定」として扱う。
var ErrKeyLost error = keyLostError{}

type keyLostError struct{}

func (keyLostError) Error() string {
	return "secrets: encryption key lost; stored credentials were discarded"
}

// Is は ErrNotFound と同一視する。
func (keyLostError) Is(target error) bool { return target == ErrNotFound }

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

// ChatGPTTokenKey は Sign in with ChatGPT のトークンのキー。kind は access／refresh／id。
// Windows 資格情報マネージャーの 1 件あたりの上限（約 2.5KB）を超える値は呼び出し側で分割する。
func ChatGPTTokenKey(kind string) string { return "hitomado/chatgpt_token/" + kind }
