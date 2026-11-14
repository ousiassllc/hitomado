package syncer

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// 新着の観測口（エージェントモード。#372）の候補の条件。
const (
	// observeMaxPerCycle は 1 周期・1 アカウントあたりに渡す候補の上限（超過分は件数だけログに残して捨てる）。
	observeMaxPerCycle = 5
	// observeMaxAge はこれより古いメッセージは候補にしない（長く止まっていたあとの同期で、古いメールを一斉に扱わない）。
	observeMaxAge = 24 * time.Hour
)

// 候補にしないラベル（受信箱のメインだけを対象にする）。
var (
	observeExcludedLabels = []string{"SENT", labelDraft, "SPAM", "TRASH",
		"CATEGORY_PROMOTIONS", "CATEGORY_SOCIAL", "CATEGORY_UPDATES", "CATEGORY_FORUMS"}
)

// NewMailCandidate は新着の観測口に渡す、1 スレッド 1 件の候補（そのスレッドで今回届いた最新のメッセージ）。
// 件名・本文・差出人は持たない（必要なら受け手がスレッドを取得する）。
type NewMailCandidate struct {
	ThreadID  string
	MessageID string
}

// NewMailObserver は差分同期で届いた新着の候補を受け取る。flush と同じタイミング（反映済みのスレッドの分。周期が失敗しても）で、
// 同期のゴルーチンから呼ばれる。同期を止めないよう、中で待たない（キューに積むだけにする）こと。
type NewMailObserver func(ctx context.Context, accountID string, cands []NewMailCandidate)

// SetNewMailObserver は新着の観測口を設定する（nil で外す）。差分同期の新着だけが対象で、初回同期・リセット同期では呼ばない。
// 候補の条件: INBOX を持ち、SENT・DRAFT・SPAM・TRASH・CATEGORY_PROMOTIONS／SOCIAL／UPDATES／FORUMS を持たない・
// 差出人がアカウント自身でない・24 時間以内・スレッドに下書き（DRAFT）が無い。1 周期・1 アカウントあたり最大 5 件。
// 未設定（nil）のときは何も変わらない。
func (s *Scheduler) SetNewMailObserver(fn NewMailObserver) {
	if fn == nil {
		s.observer.Store(nil)
		return
	}
	s.observer.Store(&fn)
}

// collectObserved は ApplyThread 直後に呼び、スレッドの新着が観測口の候補なら cyc に集める。
func (s *Scheduler) collectObserved(cyc *cycle, f fetched, newIDs []string) {
	if f.hadDraft || len(newIDs) == 0 {
		return
	}
	latest := -1
	for i, m := range f.meta.Messages {
		if slices.Contains(newIDs, m.ID) {
			latest = i
		}
	}
	if latest < 0 {
		return
	}
	m := f.meta.Messages[latest]
	if !slices.Contains(m.LabelIDs, labelInbox) || slices.ContainsFunc(m.LabelIDs, func(l string) bool { return slices.Contains(observeExcludedLabels, l) }) {
		return
	}
	if strings.EqualFold(strings.TrimSpace(m.From.Email), strings.TrimSpace(cyc.acct.Email)) {
		return
	}
	if s.clock.Now().Sub(m.InternalDate) > observeMaxAge {
		return
	}
	if len(cyc.observed) >= observeMaxPerCycle {
		cyc.observedDropped++
		return
	}
	cyc.observed = append(cyc.observed, NewMailCandidate{ThreadID: f.id, MessageID: m.ID})
}

// notifyObserver は集めた候補を観測口へ渡す。観測口の panic は同期を止めないよう、ここで受ける。
func (s *Scheduler) notifyObserver(ctx context.Context, cyc *cycle) {
	if cyc.observedDropped > 0 {
		s.log.Info("new mail candidates dropped", "account", cyc.acct.ID, "count", cyc.observedDropped)
	}
	fn := s.observer.Load()
	if fn == nil || len(cyc.observed) == 0 {
		return
	}
	defer logging.Recover(s.log, "new mail observer")
	(*fn)(ctx, cyc.acct.ID, cyc.observed)
}
