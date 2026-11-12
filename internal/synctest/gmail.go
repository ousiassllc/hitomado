package synctest

import (
	"context"
	"fmt"
	"sync"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

var _ gmail.Client = (*Gmail)(nil)

// Script はアカウント 1 件分のスクリプト。
type Script struct {
	ProfileHist string
	ProfileErr  error
	Pages       map[string]gmail.ThreadList // pageToken → 結果（"" は先頭ページ）
	ListErr     error
	Threads     map[string]gmail.ThreadMeta
	ThreadErr   map[string]error
	History     gmail.HistoryPage
	HistoryErr  error
	Labels      []gmail.Label
}

// Gmail は gmail.Client の必要メソッドだけを実装するスクリプト可能フェイク。
type Gmail struct {
	gmail.Client
	Mu       sync.Mutex
	Accts    map[string]*Script
	Calls    []string // "account:method"
	Queries  []gmail.ListQuery
	HistHook func(accountID string) // ListHistory の先頭で呼ぶ（ブロック可）
	// ThreadHook は GetThreadMetadata の先頭で呼ぶ（panic させて、並列取得の goroutine の異常を再現できる）。
	ThreadHook func(accountID, threadID string)
}

func NewGmail() *Gmail { return &Gmail{Accts: map[string]*Script{}} }

// Edit は id のスクリプトをロック下で更新する。
func (g *Gmail) Edit(id string, fn func(a *Script)) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	a, ok := g.Accts[id]
	if !ok {
		a = &Script{
			Pages:     map[string]gmail.ThreadList{},
			Threads:   map[string]gmail.ThreadMeta{},
			ThreadErr: map[string]error{},
		}
		g.Accts[id] = a
	}
	fn(a)
}

func (g *Gmail) Count(id, method string) int {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	n := 0
	for _, c := range g.Calls {
		if c == id+":"+method {
			n++
		}
	}
	return n
}

func (g *Gmail) record(id, method string) *Script {
	g.Calls = append(g.Calls, id+":"+method)
	if a, ok := g.Accts[id]; ok {
		return a
	}
	panic(fmt.Sprintf("Gmail: unscripted account %s", id))
}

func (g *Gmail) Profile(_ context.Context, id string) (gmail.Profile, error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	a := g.record(id, "Profile")
	return gmail.Profile{HistoryID: a.ProfileHist}, a.ProfileErr
}

func (g *Gmail) ListHistory(_ context.Context, id, _ string) (gmail.HistoryPage, error) {
	if g.HistHook != nil {
		g.HistHook(id)
	}
	g.Mu.Lock()
	defer g.Mu.Unlock()
	a := g.record(id, "ListHistory")
	return a.History, a.HistoryErr
}

func (g *Gmail) ListThreads(_ context.Context, id string, q gmail.ListQuery) (gmail.ThreadList, error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	a := g.record(id, "ListThreads")
	g.Queries = append(g.Queries, q)
	if a.ListErr != nil {
		return gmail.ThreadList{}, a.ListErr
	}
	return a.Pages[q.PageToken], nil
}

func (g *Gmail) GetThreadMetadata(_ context.Context, id, threadID string) (gmail.ThreadMeta, error) {
	if g.ThreadHook != nil {
		g.ThreadHook(id, threadID)
	}
	g.Mu.Lock()
	defer g.Mu.Unlock()
	a := g.record(id, "GetThreadMetadata")
	if err := a.ThreadErr[threadID]; err != nil {
		return gmail.ThreadMeta{}, err
	}
	m, ok := a.Threads[threadID]
	if !ok {
		return gmail.ThreadMeta{}, fmt.Errorf("thread %s: %w", threadID, gmail.ErrNotFound)
	}
	return m, nil
}

func (g *Gmail) ListLabels(_ context.Context, id string) ([]gmail.Label, error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	return append([]gmail.Label(nil), g.record(id, "ListLabels").Labels...), nil
}
