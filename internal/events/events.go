// Package events は Go からフロントへ発行するイベントの名前とペイロードを定める。
// ペイロードに本文・件名・宛先などの機密を載せない。
package events

// イベント名（バインディング仕様 §8）。
const (
	SyncUpdated      = "sync:updated"
	AccountStatus    = "account:status"
	NotifyOpenThread = "notify:open-thread"
	UpdateAvailable  = "update:available"
	// BeforeQuit はトレイの「終了」で発行する。フロントは未保存の下書きを保存して AppService.AckBeforeQuit を呼ぶ。
	BeforeQuit = "app:before-quit"
	// FilesDropped はウィンドウへのファイルドロップ（ペイロード {files: string[]}）。ドロップ先は data-file-drop-target 属性の要素のみ。
	FilesDropped = "files-dropped"
)

// SyncUpdatedPayload は差分同期・リセット同期でキャッシュが更新されたことを表す。
type SyncUpdatedPayload struct {
	AccountID        string   `json:"accountId"`
	ChangedThreadIDs []string `json:"changedThreadIds"`
}

// AccountStatusPayload はアカウント状態の変化。Status は active／syncing／needs_reauth／error。
type AccountStatusPayload struct {
	AccountID string `json:"accountId"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// OpenThreadPayload は通知クリックまたはトレイから開くスレッド。
type OpenThreadPayload struct {
	AccountID string `json:"accountId"`
	ThreadID  string `json:"threadId"`
}

// UpdateAvailablePayload は新バージョン検出。
type UpdateAvailablePayload struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Emitter はイベント発行の抽象。実装は main（Wails）とテストのフェイク。
type Emitter interface {
	Emit(name string, payload any)
}
