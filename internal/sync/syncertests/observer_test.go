package syncertests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// observed は新着の観測口の呼び出しの記録。
type observed struct {
	mu    sync.Mutex
	calls []observedCall
}

type observedCall struct {
	accountID string
	cands     []syncer.NewMailCandidate
	dropped   int
}

func (o *observed) observe(_ context.Context, accountID string, cands []syncer.NewMailCandidate, dropped int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, observedCall{accountID, append([]syncer.NewMailCandidate(nil), cands...), dropped})
}

func (o *observed) got() []observedCall {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]observedCall(nil), o.calls...)
}

func (o *observed) threads() []string {
	var out []string
	for _, c := range o.got() {
		for _, cand := range c.cands {
			out = append(out, cand.ThreadID)
		}
	}
	return out
}

var observeLabels = []string{"INBOX", "UNREAD", "DRAFT", "SENT", "SPAM", "TRASH", "CATEGORY_PERSONAL",
	"CATEGORY_PROMOTIONS", "CATEGORY_SOCIAL", "CATEGORY_UPDATES", "CATEGORY_FORUMS"}

func observeHarness(t *testing.T) (*harness, *observed) {
	t.Helper()
	h := newHarness(t)
	h.account("a", false, synced("100")) // 通知は無効でも、観測口は通知の設定に依らない
	h.st.Mu.Lock()
	for _, id := range observeLabels {
		h.st.Labels["a"] = append(h.st.Labels["a"], store.Label{ID: id})
	}
	h.st.Mu.Unlock()
	o := &observed{}
	h.s.SetNewMailObserver(o.observe)
	return h, o
}

// deltaWith は差分同期で threads の新着（messagesAdded）が届いた状態を作る。
func (h *harness) deltaWith(threads ...gmail.ThreadMeta) {
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{LatestHistoryID: "105"}
		for _, th := range threads {
			a.History.ChangedThreadIDs = append(a.History.ChangedThreadIDs, th.ID)
			for _, m := range th.Messages {
				a.History.Added = append(a.History.Added, gmail.HistoryAdded{MessageID: m.ID, ThreadID: th.ID, LabelIDs: m.LabelIDs})
			}
			a.Threads[th.ID] = th
		}
	})
}

func TestObserverReceivesPrimaryInboxNewMailOnly(t *testing.T) {
	h, o := observeHarness(t)
	old := msg("m-old", "t-old", "INBOX")
	old.InternalDate = h.clk.Now().Add(-25 * time.Hour)
	self := msg("m-self", "t-self", "INBOX")
	self.From.Email = "A@Example.com" // アカウント自身（大文字小文字は問わない）
	withDraft := thread("t-draft", msg("m-draft-new", "t-draft", "INBOX"), msg("d1", "t-draft", "DRAFT"))
	recent := msg("m-recent", "t-recent", "INBOX")
	recent.InternalDate = h.clk.Now().Add(-23 * time.Hour)

	h.deltaWith(
		thread("t-plain", msg("m-plain", "t-plain", "INBOX", "UNREAD")),
		thread("t-personal", msg("m-personal", "t-personal", "INBOX", "CATEGORY_PERSONAL")),
		thread("t-read", msg("m-read", "t-read", "INBOX")),
		thread("t-recent", recent),
		thread("t-promo", msg("m-promo", "t-promo", "INBOX", "CATEGORY_PROMOTIONS")),
		thread("t-social", msg("m-social", "t-social", "INBOX", "CATEGORY_SOCIAL")),
		thread("t-updates", msg("m-updates", "t-updates", "INBOX", "CATEGORY_UPDATES")),
		thread("t-forums", msg("m-forums", "t-forums", "INBOX", "CATEGORY_FORUMS")),
		thread("t-sent", msg("m-sent", "t-sent", "INBOX", "SENT")),
		thread("t-spam", msg("m-spam", "t-spam", "INBOX", "SPAM")),
		thread("t-trash", msg("m-trash", "t-trash", "INBOX", "TRASH")),
		thread("t-archived", msg("m-archived", "t-archived", "UNREAD")),
		thread("t-self", self),
		thread("t-old", old),
		withDraft,
	)
	h.s.Start("a")
	h.waitActive("a", 1)

	// 上限は 5 件なので、条件に合う 4 件（plain・personal・read・recent）が全部届き、条件に合わないものは入らない。
	assert.Equal(t, []string{"t-plain", "t-personal", "t-read", "t-recent"}, o.threads())
	calls := o.got()
	require.Len(t, calls, 1, "1 周期・1 アカウントにつき 1 回")
	assert.Equal(t, "a", calls[0].accountID)
	assert.Equal(t, syncer.NewMailCandidate{ThreadID: "t-plain", MessageID: "m-plain"}, calls[0].cands[0])
}

func TestObserverCapsCandidatesPerCycleAndReportsTheDroppedCount(t *testing.T) {
	h, o := observeHarness(t)
	var threads []gmail.ThreadMeta
	for i := range 8 {
		id := fmt.Sprintf("t%d", i)
		threads = append(threads, thread(id, msg("m"+id, id, "INBOX")))
	}
	h.deltaWith(threads...)
	h.s.Start("a")
	h.waitActive("a", 1)

	assert.Equal(t, []string{"t0", "t1", "t2", "t3", "t4"}, o.threads(), "先頭 5 件だけ渡す")
	require.Len(t, o.got(), 1)
	assert.Equal(t, 3, o.got()[0].dropped, "超過分は件数だけ渡す（ログに出すかは観測口が決める）")
}

// history の messagesAdded に無いメッセージ（ラベル変更などで初めてローカルに入った既存のメッセージ）は、候補にならない。
func TestObserverIgnoresMessagesNotInHistoryAdded(t *testing.T) {
	h, o := observeHarness(t)
	h.deltaWith(thread("t-added", msg("m-added", "t-added", "INBOX")))
	h.gm.Edit("a", func(a *synctest.Script) {
		// 受信箱へ戻されたスレッド: 変更スレッドには入るが messagesAdded は無い（ローカルには新規に保存される）。
		a.History.ChangedThreadIDs = append(a.History.ChangedThreadIDs, "t-relabeled")
		a.Threads["t-relabeled"] = thread("t-relabeled", msg("m-relabeled", "t-relabeled", "INBOX"))
	})
	h.s.Start("a")
	h.waitActive("a", 1)

	assert.Equal(t, []string{"t-added"}, o.threads())
}

func TestObserverPicksTheLatestNewMessageOfAThread(t *testing.T) {
	h, o := observeHarness(t)
	h.st.Mu.Lock()
	h.st.Threads["a"] = map[string]store.ThreadSnapshot{"t1": snap("a", "t1", ms("m1", "INBOX"))}
	h.st.Mu.Unlock()
	h.deltaWith(thread("t1", msg("m1", "t1", "INBOX"), msg("m2", "t1", "INBOX"), msg("m3", "t1", "INBOX")))
	h.s.Start("a")
	h.waitActive("a", 1)

	calls := o.got()
	require.Len(t, calls, 1)
	assert.Equal(t, []syncer.NewMailCandidate{{ThreadID: "t1", MessageID: "m3"}}, calls[0].cands, "1 スレッド 1 件。今回届いた最新のメッセージ")
}

func TestObserverIsNotCalledByInitialOrResetSync(t *testing.T) {
	// 初回同期
	h := newHarness(t)
	h.account("a", false, store.SyncState{})
	o := &observed{}
	h.s.SetNewMailObserver(o.observe)
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ProfileHist = "100"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1")}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"))
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Empty(t, o.got(), "初回同期は渡さない")

	// リセット同期（historyId 失効）
	h2 := newHarness(t)
	h2.account("a", false, synced("100"))
	o2 := &observed{}
	h2.s.SetNewMailObserver(o2.observe)
	h2.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = gmail.ErrHistoryExpired
		a.ProfileHist = "200"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1")}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"))
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	h2.s.Start("a")
	h2.waitActive("a", 1)
	assert.Equal(t, "200", h2.st.State("a").HistoryID, "リセット同期が走った")
	assert.Empty(t, o2.got(), "リセット同期は渡さない")
}

func TestObserverIsCalledEvenWhenTheCycleFailsMidway(t *testing.T) {
	h, o := observeHarness(t)
	h.deltaWith(
		thread("t1", msg("m1", "t1", "INBOX")),
		thread("t2", msg("m2", "t2", "INBOX")),
	)
	h.st.Mu.Lock()
	h.st.ApplyErr["t2"] = fmt.Errorf("disk full")
	h.st.Mu.Unlock()
	h.s.Start("a")
	h.waitStatus("a", "error", 1)

	assert.Equal(t, []string{"t1"}, o.threads(), "失敗周期でも、反映済みのスレッドの分は渡す")
}

// 観測口が未設定（nil）でも外しても、通知・同期の結果は今までと変わらない。
func TestNilObserverChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("100"))
	knownLabels(h, "a")
	o := &observed{}
	h.s.SetNewMailObserver(o.observe)
	h.s.SetNewMailObserver(nil)
	h.deltaWith(thread("t1", msg("m1", "t1", "INBOX", "UNREAD")))
	h.s.Start("a")
	h.waitActive("a", 1)

	assert.Empty(t, o.got())
	assert.Equal(t, []string{"t1"}, noticeThreads(h), "通知は変わらない")
	assert.Equal(t, "105", h.st.State("a").HistoryID)
}

// 観測口は通知の有効・無効に依らず呼ばれ、通知（Notifier）の挙動も変えない。
func TestObserverDoesNotDependOnNotifySetting(t *testing.T) {
	h, o := observeHarness(t) // NotifyEnabled=false
	h.deltaWith(thread("t1", msg("m1", "t1", "INBOX", "UNREAD")))
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, []string{"t1"}, o.threads())
	assert.Empty(t, h.nt.Got(), "通知が無効のアカウントは通知しない（観測口とは別）")
}

// 観測口が panic しても同期は止まらない（同期状態は保存され、周期は成功で終わる）。
func TestObserverPanicDoesNotBreakTheCycle(t *testing.T) {
	h, _ := observeHarness(t)
	h.s.SetNewMailObserver(func(context.Context, string, []syncer.NewMailCandidate, int) { panic("observer bug") })
	h.deltaWith(thread("t1", msg("m1", "t1", "INBOX")))
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, "105", h.st.State("a").HistoryID)
	assert.Equal(t, 1, h.skips.panicCount())
}
