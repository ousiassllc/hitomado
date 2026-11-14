// Package agentcore はエージェントモード（#372）の本体。新着メールの返信要否の判断・返信下書きの作成・利用者への質問・確認待ちの一覧。
//
// エージェントは送信しない。依存（Deps）は判断・本文づくりの AI、返信の下書きを作る入口（CreateReplyDraft だけ）、
// ストア、スレッド取得、下書きの読み取り（GetDraft だけ）を狭いインターフェースで受け、送信のメソッドを持つ型（gmail.Client や composecore.Service 全体）を持たない。
package agentcore

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

const (
	// queueCapacity は新着の待ち行列の容量。溢れた分は捨てる（件数だけログに残す）。
	queueCapacity = 50
	// retention は done／skipped／failed の項目を残す期間。Start のときに、これより古いものを削除する。
	retention = 30 * 24 * time.Hour
)

// AI はエージェントが使う AI の入口（aicore.Service が実装する）。
type AI interface {
	// Ready は AI（プロバイダ・モデル・キー／ログイン・同意）が使える状態か。
	Ready(ctx context.Context) (bool, error)
	// AgreeCurrent は AI が設定済みのとき、現在のプロバイダ・送信先への同意を記録する。未設定は AI_NOT_CONFIGURED。
	AgreeCurrent(ctx context.Context) error
	// JudgeThread は最後のメッセージに返信が要るかを判断する。
	JudgeThread(ctx context.Context, ref dto.ThreadRef) (ai.ReplyJudgment, error)
	// DraftReplyText は返信本文（プレーンテキスト）を作る。instruction は利用者の回答など（空なら指示なし）。
	DraftReplyText(ctx context.Context, ref dto.ThreadRef, instruction string) (string, error)
}

// ReplyDrafter は返信の Gmail 下書きを作る入口（composecore.Service.CreateReplyDraft）。下書きを作るだけで、送信はできない。
type ReplyDrafter interface {
	CreateReplyDraft(ctx context.Context, ref dto.ThreadRef, bodyText string) (draftID string, to []model.Address, err error)
}

// ThreadSource はスレッド（本文つき）の取得口（threadcache.Cache）。
type ThreadSource interface {
	Get(ctx context.Context, accountID, threadID string) (gmail.ThreadFull, error)
}

// Store はエージェントが使う永続化（store.Store の一部）。
type Store interface {
	GetSetting(ctx context.Context, key string) (value string, ok bool, err error)
	SetSetting(ctx context.Context, key, value string) error
	GetAccount(ctx context.Context, id string) (store.Account, error)
	GetThread(ctx context.Context, accountID, threadID string) (store.ThreadRow, error)
	CreateAgentItem(ctx context.Context, it store.AgentItem) (store.AgentItem, error)
	UpdateAgentItem(ctx context.Context, it store.AgentItem) (store.AgentItem, error)
	GetAgentItem(ctx context.Context, id string) (store.AgentItem, error)
	ListAgentItems(ctx context.Context) ([]store.AgentItem, error)
	AgentItemExists(ctx context.Context, accountID, threadID, messageID string) (bool, error)
	CountPendingAgentItems(ctx context.Context) (int, error)
	DeleteFinishedAgentItemsBefore(ctx context.Context, cutoff time.Time) (int, error)
}

// Deps は Service の依存。
type Deps struct {
	Store   Store
	Threads ThreadSource
	AI      AI
	Drafts  ReplyDrafter
	// Gmail は下書きの冒頭のプレビューを作るための読み取り口（nil 可。nil ならキャッシュに無いプレビューは空）。
	Gmail  DraftReader
	Events events.Emitter
	Clock  clock.Clock
	Logger *slog.Logger
	// Mascot は画面の端のマスコット（nil 可）。ShowMainWindow はメインウィンドウを前面に出す（nil 可）。
	Mascot         deps.MascotWindow
	ShowMainWindow func()
}

// work は待ち行列の 1 件（新着 1 通）。
type work struct{ accountID, threadID, messageID string }

// Service はエージェントモードのサービス。
type Service struct {
	d   Deps
	log *slog.Logger

	settingsMu sync.Mutex // 設定の読み書きの直列化

	queue chan work

	mu   sync.Mutex          // busy を保護
	busy map[string]struct{} // 利用者の操作（回答・やり直し）が進行中の項目 ID

	previews *previewCache // 下書きのプレビュー（メモリだけ。DB には保存しない）

	startOnce sync.Once
	wg        sync.WaitGroup
}

// New は Service を作る。
func New(d Deps) *Service {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	if d.Events == nil {
		d.Events = noopEmitter{}
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	return &Service{d: d, log: log, queue: make(chan work, queueCapacity), busy: map[string]struct{}{}, previews: newPreviewCache()}
}

// itemDTO はストアの行を DTO にする（アカウント名・下書きのプレビューは引数。To・Questions は null にしない）。
func itemDTO(it store.AgentItem, acct store.Account, draftPreview string) dto.AgentItem {
	name := acct.DisplayName
	if name == "" {
		name = acct.Email
	}
	to, questions := it.To, it.Questions
	if to == nil {
		to = []model.Address{}
	}
	if questions == nil {
		questions = []string{}
	}
	return dto.AgentItem{
		ID: it.ID, AccountID: it.AccountID, AccountName: name, AccountEmail: acct.Email, ThreadID: it.ThreadID,
		Status: it.Status, Subject: it.Subject, From: it.FromLabel, ReasonKind: it.ReasonKind, Reason: it.Reason,
		DraftID: it.DraftID, DraftPreview: draftPreview, To: to, Questions: questions, UpdatedAt: it.UpdatedAt.UnixMilli(),
	}
}

type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}
