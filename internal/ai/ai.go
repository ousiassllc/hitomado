// Package ai は AI 返信下書きのプロバイダ抽象（NF §12）を提供する。
package ai

import (
	"context"
	"errors"
	"time"
)

// AI プロバイダのエラー分類。実装は errors.Is で判定できる形で wrap して返す。
var (
	ErrUnauthorized = errors.New("ai: unauthorized") // キー不正（401／403）。ChatGPT ではコード subscription_sharing_invalid_user も含む
	ErrRateLimited  = errors.New("ai: rate limited") // レート制限（429）
	ErrFailed       = errors.New("ai: failed")       // その他のプロバイダエラー（不正な応答など）

	// ErrUsageLimit は ChatGPT 利用枠の上限超過（429 または response.failed のコード subscription_sharing_usage_limit_exceeded）。
	ErrUsageLimit = errors.New("ai: usage limit exceeded")
	// ErrNotEligible は ChatGPT 連携の対象外ユーザー（403 かつコード subscription_sharing_user_not_eligible）。
	ErrNotEligible = errors.New("ai: user not eligible")
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
