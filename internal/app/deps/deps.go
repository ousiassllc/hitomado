package deps

import (
	"context"
	"log/slog"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/threadcache"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/update"
)

// FileDialog はネイティブの保存ダイアログ。キャンセルは ok=false。
type FileDialog interface {
	SaveFile(ctx context.Context, suggestedName string) (path string, ok bool, err error)
}

// URLOpener はシステムブラウザ／メーラーで URL を開く。スキーム検証は呼び出し側（ThreadService）が行う。
type URLOpener interface {
	Open(url string) error
}

// AIFactory は設定からプロバイダを作る（テストではフェイクに差し替える）。
// provider は anthropic／openai、baseURL は openai のみ（空なら OpenAI 既定）。
type AIFactory func(provider, model, baseURL, apiKey string) (ai.Provider, error)

// Deps はサービス層が使う外部依存。具象は main.go（composition root）で組み立てる。
type Deps struct {
	Store    store.Store
	Gmail    gmail.Client
	Secrets  secrets.Store
	OAuth    oauth.Flow
	Sync     syncer.Engine
	Notifier notify.Notifier
	Update   update.Checker
	Events   events.Emitter
	Clock    clock.Clock
	Files    FileDialog
	Browser  URLOpener
	AI       AIFactory
	Logger   *slog.Logger
	DataDir  string // %APPDATA%\Hitomado
	Version  string

	// QuitAck は AppService.AckBeforeQuit から呼ばれる（フロントの未保存下書き保存の完了通知。nil 可）。
	QuitAck func()

	// UpdateTicker は RunUpdateLoop の定期確認の間隔を供給する（返す stop は終了時に呼ぶ）。
	// nil なら 24 時間ごと（NF-D03）。テストが差し替える。
	UpdateTicker func() (tick <-chan time.Time, stop func())

	// Threads は NewServices が作成する共有のスレッド本文 LRU（Deps に渡す値は不要）。
	Threads *threadcache.Cache
}
