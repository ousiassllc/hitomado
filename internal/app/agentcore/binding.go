package agentcore

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// AgentService はエージェントモード（#372）の設定・確認待ちの一覧・質問への答え（バインディング仕様 §9）。
// Wails のバインディング面で、実装は Service に委譲する。エージェントは送信しない（送信は ComposeService を人が操作したときだけ）。
type AgentService struct{ core *Service }

// NewAgentService は AgentService を作る。AI の判断・本文づくりに ai、返信の下書きづくりに drafts を使う
// （Service には、判断・本文・Ready と CreateReplyDraft、下書きの読み取り（GetDraft）だけを狭いインターフェースで渡す。送信の経路は渡さない）。
func NewAgentService(d *deps.Deps, ai AI, drafts ReplyDrafter) *AgentService {
	return &AgentService{core: New(Deps{
		Store: d.Store, Threads: d.Threads, AI: ai, Drafts: drafts, Gmail: d.Gmail,
		Events: d.Events, Clock: d.Clock, Logger: d.Logger,
		Mascot: d.Mascot, ShowMainWindow: d.ShowMainWindow,
	})}
}

// Start は古い項目の削除・マスコットの表示の反映をして、新着を処理するワーカーを起動する（冪等）。ctx の終了で止まる。
// Start・Wait・OnNewMail はアプリの起動・終了処理（run.go）と同期の観測口用で、フロントには公開しない。
//
//wails:ignore
func (s *AgentService) Start(ctx context.Context) { s.core.Start(ctx) }

// Wait はワーカーの終了を待つ（Start に渡した ctx のキャンセルのあと）。
//
//wails:ignore
func (s *AgentService) Wait() { s.core.Wait() }

// OnNewMail は同期の新着の観測口（syncer.Scheduler.SetNewMailObserver に渡す）。エージェントがオフのときは何もしない。
//
//wails:ignore
func (s *AgentService) OnNewMail(ctx context.Context, accountID string, cands []syncer.NewMailCandidate, dropped int) {
	s.core.OnNewMail(ctx, accountID, cands, dropped)
}

// GetAgentSettings はエージェントモード・マスコットの設定と、AI が使える状態かを返す。既定はすべてオフ。
func (s *AgentService) GetAgentSettings(ctx context.Context) (dto.AgentSettings, error) {
	return s.core.GetSettings(ctx)
}

// SetAgentEnabled はエージェントモードのオン・オフ。オンにするには agreed（新着メールの内容を設定済みの AI へ送ることへの同意）が真で、AI が設定済みであること。
func (s *AgentService) SetAgentEnabled(ctx context.Context, enabled, agreed bool) (dto.AgentSettings, error) {
	return s.core.SetEnabled(ctx, enabled, agreed)
}

// SetAgentMascot は画面の端のマスコット（Windows）のオン・オフ。
func (s *AgentService) SetAgentMascot(ctx context.Context, enabled bool) (dto.AgentSettings, error) {
	return s.core.SetMascot(ctx, enabled)
}

// ListAgentItems はエージェントが扱った新着（確認待ち・質問待ち・返信不要・あとで・確認済み・失敗）を新しい順に返す。
func (s *AgentService) ListAgentItems(ctx context.Context) (dto.AgentOverview, error) {
	return s.core.ListItems(ctx)
}

// AnswerAgentQuestions は質問への答えを受けて、下書きを作る（または作り直す）。answers は質問と同じ順・同じ件数。
func (s *AgentService) AnswerAgentQuestions(ctx context.Context, itemID string, answers []string) (dto.AgentItem, error) {
	return s.core.AnswerQuestions(ctx, itemID, answers)
}

// DeferAgentItem は確認待ち・質問待ちを「あとで」にする。
func (s *AgentService) DeferAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Defer(ctx, itemID)
}

// ConfirmAgentItem は利用者が下書きを開いて確認したことを記録する（確認済みにする）。
func (s *AgentService) ConfirmAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Confirm(ctx, itemID)
}

// ReconsiderAgentItem は「やっぱり返信する」。返信不要と判断したもの・失敗したものを、返信が要る前提で下書き作り（必要なら質問）からやり直す。
func (s *AgentService) ReconsiderAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Reconsider(ctx, itemID)
}

// OpenAgentPanel はメインウィンドウを前面に出し、エージェントのパネルを開かせる（マスコットの押下）。
func (s *AgentService) OpenAgentPanel(ctx context.Context) error { return s.core.OpenPanel(ctx) }
