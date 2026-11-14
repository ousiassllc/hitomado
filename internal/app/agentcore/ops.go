package agentcore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 回答 1 件の文字数の範囲（rune 数）。
const (
	minAnswerRunes = 1
	maxAnswerRunes = 1000
)

// ListItems はエージェントが扱った新着を新しい順に返す。PendingCount は確認待ち（ready）と質問待ち（asking）の合計。
func (s *Service) ListItems(ctx context.Context) (dto.AgentOverview, error) {
	items, err := s.d.Store.ListAgentItems(ctx)
	if err != nil {
		return dto.AgentOverview{}, apperr.FromInfra(err, "")
	}
	out := dto.AgentOverview{Items: make([]dto.AgentItem, 0, len(items))}
	accounts := map[string]store.Account{}
	previews := s.previewsFor(ctx, items)
	for _, it := range items {
		acct, ok := accounts[it.AccountID]
		if !ok {
			if acct, err = s.d.Store.GetAccount(ctx, it.AccountID); err != nil {
				return dto.AgentOverview{}, apperr.FromInfra(err, it.AccountID)
			}
			accounts[it.AccountID] = acct
		}
		if it.Status == store.AgentStatusReady || it.Status == store.AgentStatusAsking {
			out.PendingCount++
		}
		out.Items = append(out.Items, itemDTO(it, acct, previews[it.ID]))
	}
	return out, nil
}

// AnswerQuestions は質問への答え（質問と同じ順・同じ件数。各 1〜1000 文字）を受けて、下書きを作る（ready にする）。
// 下書きを作れなかったときはエラーを返し、質問待ちのまま残す（答えは保存しない。もう一度答えられる）。
func (s *Service) AnswerQuestions(ctx context.Context, itemID string, answers []string) (dto.AgentItem, error) {
	return s.withItem(ctx, itemID, func(item *store.AgentItem) error {
		if len(item.Questions) == 0 || (item.Status != store.AgentStatusAsking && item.Status != store.AgentStatusLater) {
			return apperr.New(apperr.Validation, "このメールには、答える質問がありません。")
		}
		if len(answers) != len(item.Questions) {
			return apperr.New(apperr.Validation, "質問の数と回答の数が合いません。")
		}
		trimmed := make([]string, len(answers))
		for i, a := range answers {
			trimmed[i] = strings.TrimSpace(a)
			if n := utf8.RuneCountInString(trimmed[i]); n < minAnswerRunes || n > maxAnswerRunes {
				return apperr.New(apperr.Validation, fmt.Sprintf("回答は %d〜%d 文字で入力してください。", minAnswerRunes, maxAnswerRunes))
			}
		}
		next := *item
		if err := s.buildDraft(ctx, &next, answersInstruction(item.Questions, trimmed)); err != nil {
			s.log.Warn("agent: draft failed", "account", item.AccountID, "thread", item.ThreadID, "kind", errKind(err))
			return err
		}
		*item = next
		return s.saveStrict(ctx, item)
	})
}

// answersInstruction は質問と利用者の回答を、返信本文を作る AI への指示にする（利用者本人が書いた回答であり、スレッド内の文章より優先する）。
func answersInstruction(questions, answers []string) string {
	var b strings.Builder
	b.WriteString("次は、返信を書くために利用者本人へ確認した質問と、その回答です。回答の内容を返信に反映してください。\n")
	for i := range questions {
		fmt.Fprintf(&b, "質問: %s\n回答: %s\n", questions[i], answers[i])
	}
	return b.String()
}

// Defer は確認待ち・質問待ちを「あとで」にする。
func (s *Service) Defer(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.withItem(ctx, itemID, func(item *store.AgentItem) error {
		if item.Status != store.AgentStatusReady && item.Status != store.AgentStatusAsking {
			return apperr.New(apperr.Validation, "確認待ち・質問待ちのものだけ「あとで」にできます。")
		}
		item.Status = store.AgentStatusLater
		return s.saveStrict(ctx, item)
	})
}

// Confirm は利用者が下書きを開いて確認したことを記録する（確認済みにする）。下書きの無いもの（質問待ちなど）は VALIDATION。
func (s *Service) Confirm(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.withItem(ctx, itemID, func(item *store.AgentItem) error {
		switch {
		case item.Status == store.AgentStatusDone:
			return nil
		case item.DraftID == "" || (item.Status != store.AgentStatusReady && item.Status != store.AgentStatusLater):
			return apperr.New(apperr.Validation, "確認できる下書きがありません。")
		}
		item.Status = store.AgentStatusDone
		return s.saveStrict(ctx, item)
	})
}

// Reconsider は「やっぱり返信する」。返信不要と判断したもの・失敗したものを、返信が要る前提でやり直す
// （もう一度 AI に見せ、決められないことがあれば質問に、無ければ下書き作りに進む）。失敗したときはエラーを返し、元の状態を保つ。
func (s *Service) Reconsider(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.withItem(ctx, itemID, func(item *store.AgentItem) error {
		if item.Status != store.AgentStatusSkipped && item.Status != store.AgentStatusFailed {
			return apperr.New(apperr.Validation, "返信不要・失敗のものだけ、やり直せます。")
		}
		ref := dto.ThreadRef{AccountID: item.AccountID, ThreadID: item.ThreadID}
		j, err := s.d.AI.JudgeThread(ctx, ref)
		if err != nil {
			s.log.Warn("agent: judge failed", "account", item.AccountID, "thread", item.ThreadID, "kind", errKind(err))
			return err
		}
		s.log.Info("agent: reconsidered", "account", item.AccountID, "thread", item.ThreadID, "needsReply", j.NeedsReply, "reasonKind", j.ReasonKind, "questions", len(j.Questions))
		next := *item
		if j.NeedsReply {
			next.ReasonKind, next.Reason = j.ReasonKind, j.Reason
		} else {
			next.ReasonKind, next.Reason = ai.ReasonOther, "返信することにしました"
		}
		if len(j.Questions) > 0 {
			next.Status, next.Questions = store.AgentStatusAsking, j.Questions
			next.DraftID, next.To = "", nil
			*item = next
			return s.saveStrict(ctx, item)
		}
		if err := s.buildDraft(ctx, &next, ""); err != nil {
			s.log.Warn("agent: draft failed", "account", item.AccountID, "thread", item.ThreadID, "kind", errKind(err))
			return err
		}
		*item = next
		return s.saveStrict(ctx, item)
	})
}

// withItem は itemID の項目に対する利用者の操作を、同じ項目の同時操作を避けて実行する。fn が item を書き換えて保存まで行い
// （saveStrict）、結果を DTO にして返す。存在しない ID は NOT_FOUND。
func (s *Service) withItem(ctx context.Context, itemID string, fn func(item *store.AgentItem) error) (dto.AgentItem, error) {
	if !s.begin(itemID) {
		return dto.AgentItem{}, apperr.New(apperr.Validation, "この項目は処理中です。しばらくしてからお試しください。")
	}
	defer s.end(itemID)
	item, err := s.d.Store.GetAgentItem(ctx, itemID)
	if errors.Is(err, store.ErrNotFound) {
		return dto.AgentItem{}, apperr.New(apperr.NotFound, "エージェントの項目が見つかりません。")
	}
	if err != nil {
		return dto.AgentItem{}, apperr.FromInfra(err, "")
	}
	if err := fn(&item); err != nil {
		return dto.AgentItem{}, apperr.FromInfra(err, item.AccountID)
	}
	acct, err := s.d.Store.GetAccount(ctx, item.AccountID)
	if err != nil {
		return dto.AgentItem{}, apperr.FromInfra(err, item.AccountID)
	}
	return itemDTO(item, acct, s.previewsFor(ctx, []store.AgentItem{item})[item.ID]), nil
}

// saveStrict は利用者の操作の結果を保存する（失敗はエラーで返す）。保存したら agent:updated を発行する。
func (s *Service) saveStrict(ctx context.Context, item *store.AgentItem) error {
	saved, err := s.d.Store.UpdateAgentItem(ctx, *item)
	if err != nil {
		return err
	}
	*item = saved
	s.emitUpdatedNow(ctx)
	return nil
}

func (s *Service) begin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.busy[id]; ok {
		return false
	}
	s.busy[id] = struct{}{}
	return true
}

func (s *Service) end(id string) {
	s.mu.Lock()
	delete(s.busy, id)
	s.mu.Unlock()
}
