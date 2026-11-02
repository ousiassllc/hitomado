package synctest

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Store は store.Store の必要メソッドだけを実装するテスト専用フェイク（未実装は nil 埋め込みで panic）。
type Store struct {
	store.Store
	Mu         sync.Mutex
	Accounts   map[string]store.Account
	Order      []string
	States     map[string]store.SyncState
	Labels     map[string][]store.Label
	Threads    map[string]map[string]store.ThreadSnapshot // account → thread → snapshot
	Contacts   map[string][]model.Address
	ApplyOrder []string
	ApplyErr   map[string]error // thread ID → エラー
	Replaced   map[string][]string
	Deleted    []string
	StatusLog  []string // "account:status:message"
	PurgeCh    chan time.Time
}

func NewStore() *Store {
	return &Store{
		Accounts: map[string]store.Account{},
		States:   map[string]store.SyncState{},
		Labels:   map[string][]store.Label{},
		Threads:  map[string]map[string]store.ThreadSnapshot{},
		Contacts: map[string][]model.Address{},
		ApplyErr: map[string]error{},
		Replaced: map[string][]string{},
		PurgeCh:  make(chan time.Time, 16),
	}
}

func (f *Store) ListAccounts(context.Context) ([]store.Account, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	out := []store.Account{}
	for _, id := range f.Order {
		out = append(out, f.Accounts[id])
	}
	return out, nil
}

func (f *Store) GetAccount(_ context.Context, id string) (store.Account, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	a, ok := f.Accounts[id]
	if !ok {
		return store.Account{}, store.ErrNotFound
	}
	return a, nil
}

func (f *Store) SetAccountStatus(_ context.Context, id, status, msg string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	a := f.Accounts[id]
	a.Status, a.StatusMessage = status, msg
	f.Accounts[id] = a
	f.StatusLog = append(f.StatusLog, id+":"+status+":"+msg)
	return nil
}

func (f *Store) GetSyncState(_ context.Context, id string) (store.SyncState, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return f.States[id], nil
}

func (f *Store) SaveSyncState(_ context.Context, s store.SyncState) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.States[s.AccountID] = s
	return nil
}

func (f *Store) ListLabels(_ context.Context, id string) ([]store.Label, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return slices.Clone(f.Labels[id]), nil
}

func (f *Store) ReplaceLabels(_ context.Context, id string, ls []store.Label) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Labels[id] = slices.Clone(ls)
	return nil
}

func (f *Store) putLocked(s store.ThreadSnapshot) store.ApplyResult {
	if f.Threads[s.AccountID] == nil {
		f.Threads[s.AccountID] = map[string]store.ThreadSnapshot{}
	}
	known := map[string]bool{}
	for _, m := range f.Threads[s.AccountID][s.ThreadID].Messages {
		known[m.ID] = true
	}
	res := store.ApplyResult{NewMessageIDs: []string{}}
	for _, m := range s.Messages {
		if !known[m.ID] {
			res.NewMessageIDs = append(res.NewMessageIDs, m.ID)
		}
	}
	f.Threads[s.AccountID][s.ThreadID] = s
	return res
}

func (f *Store) ApplyThread(_ context.Context, s store.ThreadSnapshot) (store.ApplyResult, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if err := f.ApplyErr[s.ThreadID]; err != nil {
		return store.ApplyResult{}, err
	}
	f.ApplyOrder = append(f.ApplyOrder, s.ThreadID)
	return f.putLocked(s), nil
}

func (f *Store) ReplaceInbox(_ context.Context, id string, snaps []store.ThreadSnapshot) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Replaced[id] = nil
	f.Threads[id] = nil
	for _, s := range snaps {
		f.Replaced[id] = append(f.Replaced[id], s.ThreadID)
		f.putLocked(s)
	}
	return nil
}

func (f *Store) DeleteThread(_ context.Context, id, threadID string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	delete(f.Threads[id], threadID)
	f.Deleted = append(f.Deleted, id+":"+threadID)
	return nil
}

func (f *Store) GetThread(_ context.Context, id, threadID string) (store.ThreadRow, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	s, ok := f.Threads[id][threadID]
	if !ok || len(s.Messages) == 0 {
		return store.ThreadRow{}, store.ErrNotFound
	}
	last := s.Messages[len(s.Messages)-1]
	row := store.ThreadRow{
		AccountID: id, ThreadID: threadID, Subject: s.Subject, Snippet: last.Snippet,
		LastFrom: model.Address{Name: last.FromName, Email: last.FromEmail}, MessageCount: len(s.Messages),
	}
	for _, m := range s.Messages {
		row.IsUnread = row.IsUnread || slices.Contains(m.LabelIDs, "UNREAD")
		row.InInbox = row.InInbox || slices.Contains(m.LabelIDs, "INBOX")
	}
	return row, nil
}

func (f *Store) UnreadCounts(context.Context) (map[string]int, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	out := map[string]int{}
	for acct, ts := range f.Threads {
		for _, s := range ts {
			var inbox, unread bool
			for _, m := range s.Messages {
				inbox = inbox || slices.Contains(m.LabelIDs, "INBOX")
				unread = unread || slices.Contains(m.LabelIDs, "UNREAD")
			}
			if inbox && unread {
				out[acct]++
			}
		}
	}
	return out, nil
}

func (f *Store) AddReceivedContacts(_ context.Context, id string, addrs []model.Address, _ time.Time) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Contacts[id] = append(f.Contacts[id], addrs...)
	return nil
}

func (f *Store) PurgeThreadsLeftInboxBefore(_ context.Context, cutoff time.Time) (int, error) {
	f.PurgeCh <- cutoff
	return 0, nil
}

// 以下はテストからの参照・投入用。

func (f *Store) State(id string) store.SyncState {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return f.States[id]
}

func (f *Store) Account(id string) store.Account {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return f.Accounts[id]
}

func (f *Store) Has(id, threadID string) bool {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	_, ok := f.Threads[id][threadID]
	return ok
}

func (f *Store) Put(s store.ThreadSnapshot) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.putLocked(s)
}
