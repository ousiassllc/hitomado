package deps

import (
	"context"
	"log/slog"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/threadcache"
	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/exportfile"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/update"
)

// FileDialog は保存先を利用者に選ばせる。キャンセルは ok=false。
// 返した Target の Path へ内容を書いて Commit する（Windows は選ばれたパスそのもの、Android は一時ファイルで、
// Commit が選ばれた保存先へ確定する）。呼び出し側は Target の Close を必ず呼ぶ。
type FileDialog interface {
	SaveFile(ctx context.Context, suggestedName string) (target exportfile.Target, ok bool, err error)
}

// URLOpener はシステムブラウザ／メーラーで URL を開く。スキーム検証は呼び出し側（ThreadService）が行う。
type URLOpener interface {
	Open(url string) error
}

// PathOpener はファイル・フォルダを OS の標準の方法（エクスプローラーなど）で開く。
type PathOpener interface {
	OpenPath(path string) error
}

// AIFactory は設定からプロバイダを作る（テストではフェイクに差し替える）。
// provider は anthropic／openai、baseURL は openai のみ（空なら OpenAI 既定）。
type AIFactory func(provider, model, baseURL, apiKey string) (ai.Provider, error)

// AIModelsLister はプロバイダのモデル一覧を取得する（テストではフェイクに差し替える）。
// credential は API キー（chatgpt では ChatGPT ログインのアクセストークン）。baseURL は openai のみ（空なら既定）。
type AIModelsLister func(ctx context.Context, provider, baseURL, credential string) ([]ai.ModelInfo, error)

// ClassifierFactory は TypeSafe AI（Jev。Inbox Zero の判定。#314）の API キーから Classifier を作る（テストではフェイクに差し替える）。
type ClassifierFactory func(apiKey string) ai.Classifier

// Deps はサービス層が使う外部依存。具象は main.go（composition root）で組み立てる。
type Deps struct {
	Store    store.Store
	Gmail    gmail.Client
	Secrets  secrets.Store
	OAuth    oauth.Flow
	Sync     syncer.Engine
	Notifier notify.Notifier
	// Background は OS のバックグラウンド同期・通知基盤（Android の WorkManager・通知チャネル・通知の許可）。nil は何もしない。
	Background notify.Background
	// OpenRequests は通知のタップで開くスレッドの受け渡し口（Android）。nil は常に「なし」。
	OpenRequests *notify.OpenRequests
	Update       update.Checker
	// Updates は Windows 版の更新ファイルの用意・入れ替え（NF-D03）。nil は入れ替えを行わない OS（Android・開発用の Linux）。
	Updates *update.Manager
	// Busy は送信・下書きの保存の進行中を数える（入れ替えを安全なときだけ行うため。nil 可）。
	Busy     *Busy
	Events   events.Emitter
	Clock    clock.Clock
	Files    FileDialog
	Browser  URLOpener
	Paths    PathOpener
	AI       AIFactory
	AIModels AIModelsLister
	// Classifier は Inbox Zero の判定に使う Jev の Classifier を作る。返信下書きの AI（AI）とは別の経路。
	Classifier ClassifierFactory
	ChatGPT    siwc.Auth // Sign in with ChatGPT（ログイン・更新・ログアウト）
	Logger     *slog.Logger
	DataDir    string // %APPDATA%\Hitomado
	Version    string

	// Capabilities はこの OS で提供する機能（MCP・自動更新など。Android では無効）。ゼロ値は全機能が無効。
	Capabilities capability.Capabilities

	// OAuthTracker は認可の途中でプロセスが止められたことを次の起動で検出する（Android のみ。nil＝検出しない）。
	OAuthTracker *oauth.Tracker

	// RequestQuit はアプリの終了を要求する（トレイの「終了」と同じ。未保存の下書きの保存を待ってから終了する。nil 可）。
	RequestQuit func()

	// QuitAck は AppService.AckBeforeQuit から呼ばれる（フロントの未保存下書き保存の完了通知。nil 可）。
	QuitAck func()

	// UpdateTicker は UpdateLoop の定期確認の間隔を供給する（返す stop は終了時に呼ぶ）。
	// nil なら 24 時間ごと（NF-D03）。テストが差し替える。
	UpdateTicker func() (tick <-chan time.Time, stop func())

	// Threads は NewServices が作成する共有のスレッド本文 LRU（Deps に渡す値は不要）。
	Threads *threadcache.Cache
}

// BackgroundOrNop は Background（未設定なら何もしない実装。Windows）。
func (d *Deps) BackgroundOrNop() notify.Background {
	if d.Background != nil {
		return d.Background
	}
	return notify.NopBackground{}
}

// AccountAdded はアカウントの追加が済んだあとに呼ぶ。バックグラウンド同期（Android の WorkManager）を登録し（冪等）、
// 最初のアカウント（first）の追加が済んだところで通知の許可を求める。
func (d *Deps) AccountAdded(first bool) {
	bg := d.BackgroundOrNop()
	bg.ScheduleSync(true)
	if first {
		bg.RequestPermission()
	}
}

// AccountRemoved はアカウントの削除が済んだあとに呼ぶ。そのアカウントの通知チャネルと表示中の通知を消し、
// アカウントが 0 件になったらバックグラウンド同期を解除する。
func (d *Deps) AccountRemoved(ctx context.Context, accountID string) {
	bg := d.BackgroundOrNop()
	bg.ForgetAccount(accountID)
	if rest, err := d.Store.ListAccounts(ctx); err == nil && len(rest) == 0 {
		bg.ScheduleSync(false)
	}
}
