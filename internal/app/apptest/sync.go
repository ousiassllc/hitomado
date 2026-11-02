package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// ThreadKey はスレッドの参照（FakeSync の記録用）。
type ThreadKey struct{ AccountID, ThreadID string }

// FakeSync は syncer.Engine のフェイク。呼び出しを記録する。
//
// FetchMore／RefreshThread の既定動作は実エンジンと同じ流れで、FakeGmail の内容を Store に反映する:
//   - FetchMore: sync_state.inbox_page_token で INBOX の threads.list（PageSize 件）→各スレッドのメタデータ→ApplyThread。
//     末尾で inbox_exhausted を立てる。
//   - RefreshThread: メタデータを再取得して ApplyThread。Gmail に無ければ DeleteThread して found=false。
//
// FetchMoreFn を設定すると FetchMore の既定動作の代わりに使う。
type FakeSync struct {
	mu sync.Mutex

	Store    store.Store
	Gmail    gmail.Client
	PageSize int64 // FetchMore の 1 回の取得件数。0 は 10

	RefreshErr       map[string]error // key: accountID（Refresh の結果）
	FetchMoreErr     map[string]error // key: accountID
	RefreshThreadErr map[string]error // key: Key(accountID, threadID)
	FetchMoreFn      func(ctx context.Context, accountID string) (int, error)

	Started            []string
	Stopped            []string
	Triggered          [][]string
	Refreshed          [][]string
	FetchMoreCalls     []string
	RefreshThreadCalls []ThreadKey
	ShutdownCalls      int
}

var _ syncer.Engine = (*FakeSync)(nil)

// NewFakeSync は Store／Gmail を既定動作に使う FakeSync を返す。
func NewFakeSync(st store.Store, gm gmail.Client) *FakeSync {
	return &FakeSync{
		Store: st, Gmail: gm,
		RefreshErr:       map[string]error{},
		FetchMoreErr:     map[string]error{},
		RefreshThreadErr: map[string]error{},
	}
}

// Start は記録する。
func (f *FakeSync) Start(accountID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Started = append(f.Started, accountID)
}

// Stop は記録する。
func (f *FakeSync) Stop(accountID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Stopped = append(f.Stopped, accountID)
}

// Trigger は記録する。
func (f *FakeSync) Trigger(accountIDs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Triggered = append(f.Triggered, append([]string{}, accountIDs...))
}

// Refresh は記録し、RefreshErr に従う結果を返す（空は全アカウント）。
func (f *FakeSync) Refresh(ctx context.Context, accountIDs []string) []syncer.RefreshResult {
	f.mu.Lock()
	f.Refreshed = append(f.Refreshed, append([]string{}, accountIDs...))
	f.mu.Unlock()
	ids := accountIDs
	if len(ids) == 0 {
		accounts, _ := f.Store.ListAccounts(ctx)
		for _, a := range accounts {
			ids = append(ids, a.ID)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]syncer.RefreshResult, len(ids))
	for i, id := range ids {
		out[i] = syncer.RefreshResult{AccountID: id, Err: f.RefreshErr[id]}
	}
	return out
}

// FetchMore は記録し、FetchMoreErr／FetchMoreFn／既定動作の順に従う。
func (f *FakeSync) FetchMore(ctx context.Context, accountID string) (int, error) {
	f.mu.Lock()
	f.FetchMoreCalls = append(f.FetchMoreCalls, accountID)
	err, fn, size := f.FetchMoreErr[accountID], f.FetchMoreFn, f.PageSize
	f.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if fn != nil {
		return fn(ctx, accountID)
	}
	if size <= 0 {
		size = 10
	}
	state, err := f.Store.GetSyncState(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if state.InboxExhausted {
		return 0, nil
	}
	list, err := f.Gmail.ListThreads(ctx, accountID, gmail.ListQuery{LabelIDs: []string{"INBOX"}, PageToken: state.InboxPageToken, MaxResults: size})
	if err != nil {
		return 0, err
	}
	added := 0
	for _, stub := range list.Threads {
		meta, err := f.Gmail.GetThreadMetadata(ctx, accountID, stub.ID)
		if err != nil {
			return added, err
		}
		if _, err := f.Store.ApplyThread(ctx, SnapshotFromMeta(accountID, meta)); err != nil {
			return added, err
		}
		added++
	}
	state.InboxPageToken = list.NextPageToken
	state.InboxExhausted = list.NextPageToken == ""
	return added, f.Store.SaveSyncState(ctx, state)
}

// RefreshThread は記録し、RefreshThreadErr に従うか、既定動作で DB を更新する。
func (f *FakeSync) RefreshThread(ctx context.Context, accountID, threadID string) (store.ThreadRow, bool, error) {
	f.mu.Lock()
	f.RefreshThreadCalls = append(f.RefreshThreadCalls, ThreadKey{accountID, threadID})
	err := f.RefreshThreadErr[Key(accountID, threadID)]
	f.mu.Unlock()
	if err != nil {
		return store.ThreadRow{}, false, err
	}
	meta, err := f.Gmail.GetThreadMetadata(ctx, accountID, threadID)
	if err != nil {
		if isNotFound(err) {
			return store.ThreadRow{}, false, f.Store.DeleteThread(ctx, accountID, threadID)
		}
		return store.ThreadRow{}, false, err
	}
	if _, err := f.Store.ApplyThread(ctx, SnapshotFromMeta(accountID, meta)); err != nil {
		return store.ThreadRow{}, false, err
	}
	row, err := f.Store.GetThread(ctx, accountID, threadID)
	return row, err == nil, err
}

// Shutdown は記録する。
func (f *FakeSync) Shutdown() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ShutdownCalls++
}

// StartedAccounts は Start された ID の複製を返す。
func (f *FakeSync) StartedAccounts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.Started...)
}

// StoppedAccounts は Stop された ID の複製を返す。
func (f *FakeSync) StoppedAccounts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.Stopped...)
}

// SnapshotFromMeta はスレッドのメタデータを ApplyThread の入力に変換する。
func SnapshotFromMeta(accountID string, meta gmail.ThreadMeta) store.ThreadSnapshot {
	s := store.ThreadSnapshot{AccountID: accountID, ThreadID: meta.ID, HistoryID: meta.HistoryID}
	for i, m := range meta.Messages {
		if i == 0 {
			s.Subject = m.Subject
		}
		s.HasAttachment = s.HasAttachment || m.HasAttachment
		s.Messages = append(s.Messages, store.MessageSnapshot{
			ID: m.ID, InternalDate: m.InternalDate, FromName: m.From.Name, FromEmail: m.From.Email,
			Snippet: m.Snippet, LabelIDs: m.LabelIDs,
		})
	}
	return s
}
