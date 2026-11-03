package headless

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// accountLister は store.Store のうちアカウント一覧だけ。
type accountLister interface {
	ListAccounts(ctx context.Context) ([]store.Account, error)
}

// refresher は syncer.Engine のうち手動更新だけ（アプリ本体のスケジューラも、ヘッドレス用のスケジューラも満たす）。
type refresher interface {
	Refresh(ctx context.Context, accountIDs []string) []syncer.RefreshResult
}

// RefreshAll は再接続待ち（needs_reauth）以外の全アカウントを今すぐ 1 回同期して完了を待ち、アカウントごとの結果を返す。
// 再接続待ちのアカウントは同期せず、結果にだけ載せる（呼び出し側が「再接続が必要」を知らせるため）。
func RefreshAll(ctx context.Context, st accountLister, eng refresher) ([]AccountResult, error) {
	accounts, err := st.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("headless: list accounts: %w", err)
	}
	results := make([]AccountResult, 0, len(accounts))
	var ids []string
	for _, a := range accounts {
		if a.Status == store.StatusNeedsReauth {
			results = append(results, AccountResult{ID: a.ID, Email: a.Email, Status: StatusNeedsReauth})
			continue
		}
		ids = append(ids, a.ID)
	}
	if len(ids) == 0 {
		return results, nil // Refresh に空を渡すと「全アカウント」になるため呼ばない
	}
	byID := make(map[string]store.Account, len(accounts))
	for _, a := range accounts {
		byID[a.ID] = a
	}
	for _, r := range eng.Refresh(ctx, ids) {
		results = append(results, AccountResult{ID: r.AccountID, Email: byID[r.AccountID].Email, Status: statusOf(r.Err)})
	}
	return results, nil
}

// statusOf は同期のエラーをアカウントの結果にする。再接続でしか直らないものだけを StatusNeedsReauth にする。
func statusOf(err error) AccountStatus {
	switch {
	case err == nil:
		return StatusOK
	case errors.Is(err, gmail.ErrNeedsReauth), errors.Is(err, gmail.ErrClientRejected):
		return StatusNeedsReauth
	default:
		return StatusError
	}
}

// collector は同期が見つけた新着を集める notify.Notifier（トレイが無いため SetUnread は何もしない）。
type collector struct {
	mu      sync.Mutex
	notices []notify.NewMailNotice
}

func (c *collector) NewMail(_ context.Context, n notify.NewMailNotice) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notices = append(c.notices, n)
	return nil
}

func (c *collector) SetUnread(bool) {}

func (c *collector) collected() []notify.NewMailNotice {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]notify.NewMailNotice(nil), c.notices...)
}
