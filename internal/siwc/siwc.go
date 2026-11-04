// Package siwc は OpenAI の「Sign in with ChatGPT」（オープンソース向けの ChatGPT プラン利用フロー）の
// クライアント実装。利用者自身の ChatGPT プランの範囲で Responses API を使うためのログイン・更新・ログアウトを担う。
//
// 手順は OpenAI の公式ドキュメント（https://developers.openai.com/siwc/token-sharing-open-source）に合わせる。
//   - Hitomado 自身の名前（agent_name_hint）で動的にクライアント登録する公開クライアント（client secret を持たない）。
//   - PKCE（S256）・state・nonce、リダイレクト先は 127.0.0.1 の一時ポート（/auth/callback）。ブラウザは OpenAI の公式画面で、
//     パスワードやセッション Cookie は Hitomado が一切受け取らない。
//   - Codex CLI など他アプリの認証情報は読まない・使わない（このパッケージが読み書きする資格情報は Hitomado 自身のものだけ）。
//
// トークン（アクセス・リフレッシュ・ID）は OS の資格情報ストアにだけ置く。ログ・エラー文面・戻り値に載せない
// （エラーは固定の文言とエラーコード名だけで、応答本文は含めない）。
package siwc

import (
	"context"
	"errors"
)

// ログイン・更新・ログアウトのエラー分類。errors.Is で判定できる形で返す。メッセージにトークン・応答本文は含まない。
var (
	// ErrNotSignedIn は ChatGPT にログインしていないこと。
	ErrNotSignedIn = errors.New("siwc: ChatGPT にログインしていません")
	// ErrReauthRequired はリフレッシュトークンが使えなくなり、再ログインが必要なこと（使えないトークンは消してある）。
	ErrReauthRequired = errors.New("siwc: ChatGPT に再ログインが必要です")
	// ErrDenied は OpenAI の画面で許可されなかった（access_denied）こと。
	ErrDenied = errors.New("siwc: ログインが許可されませんでした")
	// ErrPlanNotGranted はログインはできたが ChatGPT プランの利用（chatgpt.tokens.use.direct）が許可されなかったこと。
	// トークンは保存しない。次のログインでは同意画面を求め直す。
	ErrPlanNotGranted = errors.New("siwc: ChatGPT プランの利用が許可されませんでした")
	// ErrCancelled はログインを中止したこと。
	ErrCancelled = errors.New("siwc: ログインを中止しました")
	// ErrTimeout はブラウザでの操作が時間内に完了しなかったこと。
	ErrTimeout = errors.New("siwc: ログインが時間内に完了しませんでした")
	// ErrBusy は別のログインが進行中なこと。
	ErrBusy = errors.New("siwc: 別のログインが進行中です")
	// ErrAccountMismatch は選んだ登録と別の ChatGPT アカウントでログインされたこと（保存しない）。
	ErrAccountMismatch = errors.New("siwc: 選んだ登録とは別の ChatGPT アカウントです")
	// ErrUnknownAccount は保存されていない ChatGPT アカウントを指定したこと。
	ErrUnknownAccount = errors.New("siwc: 登録されていない ChatGPT アカウントです")
	// ErrAccountInUse はログイン中のアカウントの登録は忘れられないこと（先にログアウトする）。
	ErrAccountInUse = errors.New("siwc: ログイン中のアカウントは削除できません")
	// ErrInvalidResponse は OpenAI からの応答（コールバック・トークン・ID トークン）が検証を通らなかったこと。
	ErrInvalidResponse = errors.New("siwc: OpenAI からの応答を検証できません")
	// ErrFailed はその他の失敗（サーバーエラー・設定不備・資格情報ストアの失敗など）。
	ErrFailed = errors.New("siwc: ChatGPT のログイン処理に失敗しました")
)

// Account は保存済みの ChatGPT アカウント（クライアント登録）1 件。トークンは含まない。
type Account struct {
	// ID は発行された client_id。同じメールアドレスでも別の登録なら別の ID になる。
	ID       string
	Email    string // 検証済み ID トークンのメールアドレス（表示用）
	SignedIn bool   // ログイン中のセッションがあるか
	Active   bool   // 使用中のアカウントか
}

// Status はログイン状態。トークンは含まない。
type Status struct {
	// SignedIn・Email は使用中のアカウントのログイン状態とメールアドレス。
	SignedIn bool
	Email    string
	// Accounts は保存済みのアカウント（登録順）。
	Accounts []Account
}

// SignInOptions はログインの指定。
type SignInOptions struct {
	// AccountID が空なら、新しく登録する（別の ChatGPT アカウントの追加）。
	// 指定したときは、その登録の client_id で再ログインする（同じアカウントであること）。
	// どちらも、ID トークンを検証して保存を終えるまで、使用中のアカウントと保存済みのトークンは変えない。
	AccountID string
}

// Auth は ChatGPT ログインの操作面（aicore が使う）。実装は *Manager。
type Auth interface {
	// SignIn はブラウザでのログインの完了・中止・タイムアウトまでブロックする。ctx のキャンセルは ErrCancelled。
	// 成功したアカウントが使用中になる。
	SignIn(ctx context.Context, opts SignInOptions) (Status, error)
	// CancelSignIn は進行中のログインを中止する（SignIn は ErrCancelled で戻る）。無ければ何もしない。
	CancelSignIn()
	// Status は現在のログイン状態と保存済みのアカウントを返す（通信しない）。
	Status(ctx context.Context) (Status, error)
	// SelectAccount は使用中のアカウントを切り替える。ログイン中のアカウントだけ選べる（未登録は ErrUnknownAccount、
	// ログアウト済みは ErrNotSignedIn）。ほかのアカウントのトークンと登録は変えない。
	SelectAccount(ctx context.Context, accountID string) error
	// AccessToken は使用中のアカウントの有効なアクセストークンを返す。期限が近ければ更新する。
	// 未ログインは ErrNotSignedIn、更新できず再ログインが必要なら ErrReauthRequired。
	AccessToken(ctx context.Context) (string, error)
	// ForceRefresh は期限に関わらず使用中のアカウントのトークンを更新して新しいアクセストークンを返す（401 を受けたときに 1 度だけ使う）。
	ForceRefresh(ctx context.Context) (string, error)
	// SignOut は指定したアカウントだけ、リフレッシュトークンの失効を試みてから、そのローカルのトークンを消す。
	// ほかのアカウントのトークンと登録は残り、このアカウントの登録（client_id・ホスト ID）も残す。
	// revoked は OpenAI 側の失効を確認できたか。消せなかったときだけ err が返る。
	SignOut(ctx context.Context, accountID string) (revoked bool, err error)
	// RemoveAccount はログアウト済みのアカウントの登録を忘れる（ログイン中は ErrAccountInUse）。
	RemoveAccount(ctx context.Context, accountID string) error
}

var _ Auth = (*Manager)(nil)
