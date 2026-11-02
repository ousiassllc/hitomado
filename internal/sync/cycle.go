package syncer

import (
	"context"
	"errors"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/store"
)

// UI に出すステータスメッセージ。機密（トークン・本文・宛先）を含めない。
const (
	msgNeedsReauth = "再接続が必要です"
	msgOffline     = "ネットワークに接続できません"
	msgRateLimited = "Gmail のリクエスト制限中です。次の周期で再試行します"
	msgFailed      = "同期に失敗しました"
)

// statusSyncing は DB に保存しない一時状態（account:status のみ）。
const statusSyncing = "syncing"

// cycle は 1 周期の途中経過。失敗しても反映済みの分は通知・イベントへ出すために集める。
type cycle struct {
	acct     store.Account
	changed  []string // 反映したスレッド ID（削除含む）。重複なし
	newMsgs  map[string]map[string]struct{}
	contacts []model.Address
	notices  []notify.NewMailNotice
}

func newCycle(acct store.Account) *cycle {
	return &cycle{acct: acct, newMsgs: map[string]map[string]struct{}{}}
}

func (c *cycle) markChanged(threadID string) {
	for _, id := range c.changed {
		if id == threadID {
			return
		}
	}
	c.changed = append(c.changed, threadID)
}

func (c *cycle) isNew(threadID, messageID string) bool {
	_, ok := c.newMsgs[threadID][messageID]
	return ok
}

// syncAccount は 1 周期（状態 syncing→active／needs_reauth／error）を実行する。呼び出し側が a.syncMu を保持する。
func (s *Scheduler) syncAccount(ctx context.Context, a *account) error {
	acct, err := s.store.GetAccount(ctx, a.id)
	if err != nil {
		return fmt.Errorf("syncer: get account: %w", err)
	}
	st, err := s.store.GetSyncState(ctx, a.id)
	if err != nil {
		return fmt.Errorf("syncer: get sync state: %w", err)
	}
	s.emitStatus(a.id, statusSyncing, "")
	cyc := newCycle(acct)
	next, err := s.runPhase(ctx, cyc, st)
	if err == nil {
		next.LastSyncedAt = s.clock.Now()
		next.LastError = ""
		if err = s.store.SaveSyncState(ctx, next); err != nil {
			err = fmt.Errorf("syncer: save sync state: %w", err)
		}
	}
	// 通知・イベントは失敗時も、反映済みの分を出す。
	s.flush(ctx, cyc)
	if err == nil {
		s.publishStatus(ctx, a, acct, store.StatusActive, "")
		return nil
	}
	if ctx.Err() != nil {
		return err // Stop／Shutdown による中断。状態は変えない
	}
	status, msg := classify(err)
	st.LastError = msg
	if saveErr := s.store.SaveSyncState(ctx, st); saveErr != nil {
		s.log.Warn("save sync state failed", "account", a.id, "err", saveErr)
	}
	if status == store.StatusNeedsReauth {
		a.paused.Store(true)
	}
	s.publishStatus(ctx, a, acct, status, msg)
	return err
}

// runPhase は同期状態から初回・リセット・差分のどれを行うか決めて実行し、保存すべき新しい状態を返す。
func (s *Scheduler) runPhase(ctx context.Context, cyc *cycle, st store.SyncState) (store.SyncState, error) {
	switch {
	case !st.InitialSyncDone:
		return s.initialSync(ctx, cyc, st)
	case st.HistoryID == "":
		return s.resetSync(ctx, cyc, st)
	}
	next, err := s.deltaSync(ctx, cyc, st)
	if errors.Is(err, gmail.ErrHistoryExpired) {
		return s.resetSync(ctx, cyc, st)
	}
	return next, err
}

// classify はエラーを保存する状態と UI 向けメッセージに写像する。
func classify(err error) (status, msg string) {
	switch {
	case errors.Is(err, gmail.ErrNeedsReauth):
		return store.StatusNeedsReauth, msgNeedsReauth
	case errors.Is(err, gmail.ErrOffline):
		return store.StatusError, msgOffline
	case errors.Is(err, gmail.ErrRateLimited):
		return store.StatusError, msgRateLimited
	default:
		return store.StatusError, msgFailed
	}
}

func (s *Scheduler) emitStatus(accountID, status, msg string) {
	s.events.Emit(events.AccountStatus, events.AccountStatusPayload{AccountID: accountID, Status: status, Message: msg})
}

// publishStatus は状態が変わっていれば DB に保存し、account:status を発行する。
// キャンセル後（Stop／Shutdown）でも書き込めるよう ctx のキャンセルを切り離す。
func (s *Scheduler) publishStatus(ctx context.Context, a *account, cur store.Account, status, msg string) {
	if cur.Status != status || cur.StatusMessage != msg {
		if err := s.store.SetAccountStatus(context.WithoutCancel(ctx), a.id, status, msg); err != nil {
			s.log.Warn("set account status failed", "account", a.id, "err", err)
		}
	}
	s.emitStatus(a.id, status, msg)
}

// flush は 1 周期の最後に、新着通知・連絡先・トレイの未読・sync:updated を出す。
func (s *Scheduler) flush(ctx context.Context, cyc *cycle) {
	ctx = context.WithoutCancel(ctx)
	for _, n := range cyc.notices {
		if err := s.notifier.NewMail(ctx, n); err != nil {
			s.log.Warn("notify failed", "account", n.AccountID, "err", err)
		}
	}
	if len(cyc.contacts) > 0 {
		if err := s.store.AddReceivedContacts(ctx, cyc.acct.ID, cyc.contacts, s.clock.Now()); err != nil {
			s.log.Warn("add contacts failed", "account", cyc.acct.ID, "err", err)
		}
	}
	s.updateUnread(ctx)
	if len(cyc.changed) > 0 {
		s.events.Emit(events.SyncUpdated, events.SyncUpdatedPayload{AccountID: cyc.acct.ID, ChangedThreadIDs: cyc.changed})
	}
}

// updateUnread は全アカウント合計の未読有無をトレイに反映する。
func (s *Scheduler) updateUnread(ctx context.Context) {
	counts, err := s.store.UnreadCounts(ctx)
	if err != nil {
		s.log.Warn("unread counts failed", "err", err)
		return
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	s.notifier.SetUnread(total > 0)
}
