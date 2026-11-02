// Package model は domain 層とインフラ層が共有する純粋なデータ型を定義する。
package model

import "time"

// Mode は作成セッションの種別。
type Mode string

// 作成セッションの種別。
const (
	ModeReply    Mode = "reply"
	ModeReplyAll Mode = "reply_all"
	ModeForward  Mode = "forward"
	ModeNew      Mode = "new"
)

// Address はメールアドレスと表示名。Email は小文字正規化済みを期待する。
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// AttachmentInfo は受信メッセージの添付のメタデータ（データは含まない）。
type AttachmentInfo struct {
	ID       string `json:"id"` // Gmail の attachmentId
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

// SourceMessage は返信・転送の元になるメッセージ（メモリ上のみ。永続化しない）。
type SourceMessage struct {
	ID          string
	ThreadID    string
	MessageID   string   // Message-ID ヘッダ（山括弧込み）
	References  []string // 元メッセージの References ヘッダ（山括弧込み）
	From        Address
	ReplyTo     []Address
	To          []Address
	Cc          []Address
	Subject     string
	Date        time.Time
	BodyHTML    string // 未サニタイズ。空ならプレーンテキストのみ
	BodyText    string
	Attachments []AttachmentInfo
}

// Recipients は返信宛先の計算結果。
type Recipients struct {
	To []Address
	Cc []Address
}

// OutgoingAttachment は送信する添付（Data は送信時に読み込む）。
type OutgoingAttachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// OutgoingMessage は MIME 組立の入力。From は必ず accounts.email から生成する。
type OutgoingMessage struct {
	From        Address
	To          []Address
	Cc          []Address
	Bcc         []Address
	Subject     string
	BodyHTML    string
	BodyText    string // 空なら BodyHTML から生成する
	InReplyTo   string
	References  []string
	Attachments []OutgoingAttachment
	Date        time.Time
}
