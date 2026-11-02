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
