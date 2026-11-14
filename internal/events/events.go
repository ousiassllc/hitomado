// Package events は Go からフロントへ発行するイベントの名前とペイロードを定める。
// ペイロードに本文・件名・宛先などの機密を載せない。
package events

// イベント名（バインディング仕様 §8）。
const (
	SyncUpdated      = "sync:updated"
	AccountStatus    = "account:status"
	NotifyOpenThread = "notify:open-thread"
	UpdateAvailable  = "update:available"
	// UpdateStatus は自動アップデートの進み（ダウンロード中・入れ替え待ち・失敗・なし）が変わったとき（ペイロード UpdateStatusPayload）。
	UpdateStatus = "update:status"
	// BeforeQuit はトレイの「終了」で発行する。フロントは未保存の下書きを保存して AppService.AckBeforeQuit を呼ぶ。
	BeforeQuit = "app:before-quit"
	// FilesDropped はウィンドウへのファイルドロップ（ペイロード {files: string[]}）。ドロップ先は data-file-drop-target 属性の要素のみ。
	FilesDropped = "files-dropped"
	// AgentUpdated はエージェントモードの確認待ち・質問待ちの件数や設定が変わったとき（ペイロード AgentUpdatedPayload）。
	AgentUpdated = "agent:updated"
	// AgentOpenPanel はマスコットの押下などで、メインウィンドウのエージェントのパネルを開かせる（ペイロードなし）。
	AgentOpenPanel = "agent:open-panel"
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

// OpenThreadPayload は通知クリックで開くスレッド。
type OpenThreadPayload struct {
	AccountID string `json:"accountId"`
	ThreadID  string `json:"threadId"`
}

// UpdateAvailablePayload は新バージョン検出。
type UpdateAvailablePayload struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// 自動アップデートの段階（UpdateStatusPayload.Stage）。
const (
	UpdateStageNone        = "none"
	UpdateStageDownloading = "downloading"
	UpdateStageReady       = "ready" // ダウンロード・照合が済み、入れ替え待ち（「再起動して更新」を出す）
	UpdateStageFailed      = "failed"
)

// UpdateStatusPayload は自動アップデートの状態。Message は画面に出してよい日本語の文面（失敗・注意があるときだけ。URL・パスを含めない）。
type UpdateStatusPayload struct {
	Stage   string `json:"stage"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
}

// AgentUpdatedPayload は確認待ち・質問待ちの合計件数（本文・件名は載せない）と、エージェント・マスコットの設定。
type AgentUpdatedPayload struct {
	PendingCount  int  `json:"pendingCount"`
	Enabled       bool `json:"enabled"`
	MascotEnabled bool `json:"mascotEnabled"`
}

// Emitter はイベント発行の抽象。実装は main（Wails）とテストのフェイク。
type Emitter interface {
	Emit(name string, payload any)
}
