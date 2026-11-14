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

// MaxQuestionRunes はスレッドへの AI 質問の最大文字数（rune 数。F-18。#207）。
const MaxQuestionRunes = 2000

// スレッドへの AI 質問の会話（history）の上限（F-18。#207）。Go は黙って切り詰めず、超えたら VALIDATION にする。
const (
	MaxHistoryTurns = 20    // 10 往復
	MaxHistoryRunes = 20000 // text の合計（rune 数）
)

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
	// Attachments は添付のファイル名とサイズ（スレッドの AI 要約・質問のみ。中身は送らない）。返信下書きでは使わない。
	Attachments []AttachmentRef
}

// AttachmentRef は添付のファイル名とサイズ（バイト）。
type AttachmentRef struct {
	Filename string
	Size     int64
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

// ThreadInput はスレッドの AI 要約（F-17）の入力。Messages は呼び出し側が全メッセージ・合計 40,000 文字に収める（時系列昇順）。
type ThreadInput struct {
	// AccountDisplayName・AccountEmail は受信アカウントの利用者（「求められている対応」の主語）。空でもよい。
	AccountDisplayName string
	AccountEmail       string
	// Effort は ReplyInput.Effort と同じ（空は要求に載せない）。
	Effort   string
	Messages []ContextMessage // 時系列昇順
}

// Role は ThreadTurn.Role の値。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// ThreadTurn はスレッドへの質問の会話 1 件。
type ThreadTurn struct {
	Role string // RoleUser／RoleAssistant
	Text string
}

// ThreadQuestionInput はスレッドへの AI 質問（F-18）の入力。History は user から始まり assistant で終わる交互の会話
// （空でもよい。検証は呼び出し側）。Question は今回の質問。
type ThreadQuestionInput struct {
	ThreadInput
	History  []ThreadTurn
	Question string
}

// Provider は AI プロバイダ。ログに本文・キーを出さないこと。
type Provider interface {
	// GenerateReply は返信本文（プレーンテキスト）を返す。タイムアウトは呼び出し側が ctx で与える（60 秒）。
	GenerateReply(ctx context.Context, in ReplyInput) (string, error)
	// SummarizeThread はスレッドの要約（日本語のプレーンテキストの箇条書き）を返す（F-17）。
	SummarizeThread(ctx context.Context, in ThreadInput) (string, error)
	// AskAboutThread はスレッドの内容と会話を文脈に、質問への回答（日本語のプレーンテキスト）を返す（F-18）。
	AskAboutThread(ctx context.Context, in ThreadQuestionInput) (string, error)
	// JudgeReply は最後のメッセージに返信が要るかを判断する（エージェントモード。#372）。
	// 応答が JSON として読めなければ ErrFailed。
	JudgeReply(ctx context.Context, in ReplyJudgeInput) (ReplyJudgment, error)
}
