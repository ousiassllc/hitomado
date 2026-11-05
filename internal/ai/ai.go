// Package ai は AI 返信下書きのプロバイダ抽象（NF §12）を提供する。
package ai

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// MaxInstructionRunes は「AI への指示」の最大文字数（rune 数。#109）。
const MaxInstructionRunes = 2000

// AI プロバイダのエラー分類。実装は errors.Is で判定できる形で wrap して返す。
var (
	ErrUnauthorized = errors.New("ai: unauthorized") // キー不正（401／403）。ChatGPT ではコード subscription_sharing_invalid_user も含む
	ErrRateLimited  = errors.New("ai: rate limited") // レート制限（429）
	ErrFailed       = errors.New("ai: failed")       // その他のプロバイダエラー（不正な応答など）
	// ErrRejected はプロバイダが要求を 400／422 で拒否したこと（ErrFailed でもある）。エフォートを載せた要求では、エフォートが原因のことがある。
	ErrRejected = fmt.Errorf("%w: rejected", ErrFailed)

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
	// Instruction は利用者が書いた「AI への指示」（#109）。空なら指示なし（従来と同じ要求）。スレッド本文とは別枠（system 側）で渡す。
	Instruction string
	// Effort はモデルのエフォート（#111）。空は「モデルの既定」で、要求に載せない（従来と同じ要求）。
	// メール内容ではなく生成のパラメータなので、同意の単位・送信先は変わらない。範囲の検査は呼び出し側（ValidEffort）。
	Effort   string
	Messages []ContextMessage // 時系列昇順
}

// Provider は AI プロバイダ。ログに本文・キーを出さないこと。
type Provider interface {
	// GenerateReply は返信本文（プレーンテキスト）を返す。タイムアウトは呼び出し側が ctx で与える（60 秒）。
	GenerateReply(ctx context.Context, in ReplyInput) (string, error)
}
