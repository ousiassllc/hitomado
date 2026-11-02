// Package ai は AI 返信下書きのプロバイダ抽象（NF §12）を提供する。
package ai

import (
	"context"
	"time"
)

// ContextMessage はプロンプトに含めるスレッド内のメッセージ（プレーンテキスト化済み）。
type ContextMessage struct {
	From string
	Date time.Time
	Text string
}

// ReplyInput は返信下書き生成の入力。送信コンテキストは直近 5 メッセージ・合計 20,000 文字以内（呼び出し側が切り詰める）。
type ReplyInput struct {
	AccountDisplayName string
	SignatureText      string
	Messages           []ContextMessage // 時系列昇順
}

// Provider は AI プロバイダ。ログに本文・キーを出さないこと。
type Provider interface {
	// GenerateReply は返信本文（プレーンテキスト）を返す。タイムアウトは呼び出し側が ctx で与える（60 秒）。
	GenerateReply(ctx context.Context, in ReplyInput) (string, error)
}
