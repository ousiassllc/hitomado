package dto

// InboxQuery は受信箱の取得条件。AccountIDs が空ならすべて。Limit は 1〜100（既定 50）。
type InboxQuery struct {
	AccountIDs []string `json:"accountIds"`
	UnreadOnly bool     `json:"unreadOnly"`
	Limit      int      `json:"limit"`
	Cursor     string   `json:"cursor,omitempty"`
}

// SearchQuery は Gmail 検索クエリ（そのまま渡す）。
type SearchQuery struct {
	Q          string   `json:"q"`
	AccountIDs []string `json:"accountIds"`
	Limit      int      `json:"limit"`
	Cursor     string   `json:"cursor,omitempty"`
}

// RefreshResult はアカウントごとの手動更新の結果。
type RefreshResult struct {
	AccountID string         `json:"accountId"`
	OK        bool           `json:"ok"`
	Error     map[string]any `json:"error,omitempty"`
}

// RefreshResponse は Refresh の戻り値。
type RefreshResponse struct {
	Results []RefreshResult `json:"results"`
}
