package dto

// InboxFilter は保存する受信箱フィルタ。
type InboxFilter struct {
	AccountIDs []string `json:"accountIds"`
	UnreadOnly bool     `json:"unreadOnly"`
}

// AppInfo はアプリ情報。
type AppInfo struct {
	Version            string `json:"version"`
	UpdateCheckEnabled bool   `json:"updateCheckEnabled"`
	DataDir            string `json:"dataDir"`
}

// UpdateInfo は新バージョン確認の結果。
type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	URL       string `json:"url,omitempty"`
}

// AccountPatch は UpdateAccount の部分更新（nil は変更なし）。
type AccountPatch struct {
	DisplayName   *string `json:"displayName,omitempty"`
	Color         *string `json:"color,omitempty"`
	NotifyEnabled *bool   `json:"notifyEnabled,omitempty"`
}

// ClientError はフロントが Go のログへ送るエラー報告。メール本文・件名・宛先は送らない
// （画面側が組み立てるのはエラーの種類・メッセージ・スタック・エラーコードだけ。Go 側でも伏せて長さを切る）。
type ClientError struct {
	Kind      string `json:"kind"` // error / unhandledrejection / vue / binding
	Message   string `json:"message"`
	Stack     string `json:"stack,omitempty"`
	Source    string `json:"source,omitempty"`    // error イベントの発生元（ファイル:行:列）
	Code      string `json:"code,omitempty"`      // binding のとき AppError.code
	AccountID string `json:"accountId,omitempty"` // binding のとき AppError.accountId
}

// ExportLogsResult はログの書き出し結果。キャンセルは Saved=false。
type ExportLogsResult struct {
	Saved bool   `json:"saved"`
	Path  string `json:"path,omitempty"`
}

// MCPConfig は内蔵 MCP サーバーの設定。既定はオフ。AllowNonLoopback は loopback 以外のアドレス（tailnet など）で
// 待ち受けるときだけ true にする。
type MCPConfig struct {
	Enabled          bool   `json:"enabled"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	AllowNonLoopback bool   `json:"allowNonLoopback"`
}

// MCPStatus は設定と待ち受けの状態。トークンの値は含まない（発行直後の IssueMCPToken だけが返す）。
type MCPStatus struct {
	MCPConfig
	HasToken  bool   `json:"hasToken"`
	Listening bool   `json:"listening"`
	Endpoint  string `json:"endpoint"`        // MCP クライアントに設定する URL
	Error     string `json:"error,omitempty"` // 待ち受けに失敗した理由
}

// MCPTokenResult は IssueMCPToken の結果。Token はこのとき 1 回だけ返す（以後は取り出せない）。
type MCPTokenResult struct {
	Token  string    `json:"token"`
	Status MCPStatus `json:"status"`
}
