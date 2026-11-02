package dto

import "github.com/ousiassllc/hitomado/internal/domain/model"

// ThreadMessage はスレッド内のメッセージ。BodyHTML は未サニタイズ（サニタイズはフロント）。
type ThreadMessage struct {
	ID              string                 `json:"id"`
	From            model.Address          `json:"from"`
	To              []model.Address        `json:"to"`
	Cc              []model.Address        `json:"cc"`
	Date            int64                  `json:"date"`
	IsUnread        bool                   `json:"isUnread"`
	BodyHTML        string                 `json:"bodyHtml,omitempty"` // cid 画像は data: URL に置換済み
	BodyText        string                 `json:"bodyText,omitempty"` // HTML がない場合のみ
	HasRemoteImages bool                   `json:"hasRemoteImages"`
	Attachments     []model.AttachmentInfo `json:"attachments"`
}

// ThreadDetail は GetThread の結果。Messages は下書きを除く時系列昇順。
type ThreadDetail struct {
	Account  Account         `json:"account"`
	ThreadID string          `json:"threadId"`
	Subject  string          `json:"subject"`
	Messages []ThreadMessage `json:"messages"`
	DraftID  string          `json:"draftId,omitempty"`
	Labels   []Label         `json:"labels"`
}

// SaveAttachmentResult は添付保存の結果。キャンセルは Saved=false。
type SaveAttachmentResult struct {
	Saved bool   `json:"saved"`
	Path  string `json:"path,omitempty"`
}
