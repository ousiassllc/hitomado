package dto

import "github.com/ousiassllc/hitomado/internal/domain/model"

// OpenComposeRequest は作成セッションの開始要求。
type OpenComposeRequest struct {
	Mode            string     `json:"mode"` // reply / reply_all / forward / new
	Ref             *ThreadRef `json:"ref,omitempty"`
	SourceMessageID string     `json:"sourceMessageId,omitempty"`
	AccountID       string     `json:"accountId,omitempty"` // new のみ
	DraftID         string     `json:"draftId,omitempty"`
}

// ComposeContent はフロントが都度渡す入力内容。from を持たない（送信元の指定経路を作らない）。
type ComposeContent struct {
	To       []model.Address `json:"to"`
	Cc       []model.Address `json:"cc"`
	Bcc      []model.Address `json:"bcc"`
	Subject  string          `json:"subject"`
	BodyHTML string          `json:"bodyHtml"` // 署名ブロックは <div data-hitomado-signature> で囲む
}

// ComposeAttachment は添付一覧の 1 件。Source は local / forwarded。
type ComposeAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Source   string `json:"source"`
}

// ComposeSession は作成セッション。From は未選択なら nil。
type ComposeSession struct {
	SessionID    string              `json:"sessionId"`
	Mode         string              `json:"mode"`
	From         *model.Address      `json:"from,omitempty"`
	AccountID    string              `json:"accountId,omitempty"`
	SenderLocked bool                `json:"senderLocked"`
	Content      ComposeContent      `json:"content"`
	Attachments  []ComposeAttachment `json:"attachments"`
	DraftID      string              `json:"draftId,omitempty"`
	// AttachmentError は下書きの添付を復元できなかったとき（合計が 25MB を超える等）の理由（apperr JSON 形）。
	// 本文・宛先などは復元済みで、添付だけが空になる。
	AttachmentError map[string]any `json:"attachmentError,omitempty"`
}

// SaveDraftResult は下書き保存の結果。
type SaveDraftResult struct {
	DraftID string `json:"draftId"`
	SavedAt int64  `json:"savedAt"`
}

// SendOptions は PrepareSend のオプション。
type SendOptions struct {
	ArchiveAfterSend bool `json:"archiveAfterSend"`
}

// PreviewFrom は確認ダイアログの送信元。
type PreviewFrom struct {
	model.Address
	AccountID string `json:"accountId"`
	Color     string `json:"color"`
}

// SendWarning は送信前の警告。Code は OTHER_OWN_ACCOUNT / EMPTY_SUBJECT / EMPTY_BODY。
type SendWarning struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Emails  []string `json:"emails,omitempty"` // OTHER_OWN_ACCOUNT のみ
}

// SendPreview は PrepareSend の結果。ConfirmToken は 5 分有効・1 回限り。
type SendPreview struct {
	ConfirmToken    string          `json:"confirmToken"`
	ExpiresAt       int64           `json:"expiresAt"`
	From            PreviewFrom     `json:"from"`
	To              []model.Address `json:"to"`
	Cc              []model.Address `json:"cc"`
	Bcc             []model.Address `json:"bcc"`
	Subject         string          `json:"subject"`
	AttachmentCount int             `json:"attachmentCount"`
	BodyPreview     string          `json:"bodyPreview"` // プレーンテキスト（改行を保つ）、約 600 文字。署名・引用は含めない
	Warnings        []SendWarning   `json:"warnings"`
}

// SendResult は Send の結果。アーカイブ失敗時は Archived=false・ArchiveError に理由（apperr JSON 形）。
type SendResult struct {
	MessageID    string         `json:"messageId"`
	ThreadID     string         `json:"threadId"`
	Archived     bool           `json:"archived"`
	ArchiveError map[string]any `json:"archiveError,omitempty"`
}
