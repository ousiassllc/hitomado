package apptests

import (
	"cmp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func ref(acc, thr string) dto.ThreadRef { return dto.ThreadRef{AccountID: acc, ThreadID: thr} }

func TestMarkReadAndUnread(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD"))
	e.PutThread(t, b.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX", "UNREAD"))

	out, err := svc.MarkRead(t.Context(), []dto.ThreadRef{ref(a.ID, "t1"), ref(b.ID, "t2")}, true)
	require.NoError(t, err)
	require.Len(t, out, 2)
	for _, s := range out {
		assert.False(t, s.IsUnread)
	}
	row, err := e.Store.GetThread(t.Context(), a.ID, "t1")
	require.NoError(t, err)
	assert.False(t, row.IsUnread, "DB も更新される")
	assert.Equal(t, []string{"UNREAD"}, e.Gmail.Modified[0].Remove)

	out, err = svc.MarkRead(t.Context(), []dto.ThreadRef{ref(a.ID, "t1")}, false)
	require.NoError(t, err)
	assert.True(t, out[0].IsUnread)

	out, err = svc.MarkRead(t.Context(), nil, true)
	require.NoError(t, err)
	assert.NotNil(t, out)
	_, err = svc.MarkRead(t.Context(), []dto.ThreadRef{{AccountID: a.ID}}, true)
	_ = requireCode(t, apperr.Validation, err)
}

func failedRefs(t *testing.T, ae *apperr.Error) []map[string]string {
	t.Helper()
	v, ok := ae.Details["failedRefs"].([]map[string]string)
	require.True(t, ok, "details.failedRefs がある: %#v", ae.Details)
	return v
}

// アカウント単位のワーカーが panic しても、プロセスは落ちず、そのアカウントの ref は失敗（INTERNAL）として failedRefs に載り、
// ほかのアカウントの操作は反映される。
func TestApplyRecoversPanicInAccountWorker(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD"))
	e.PutThread(t, b.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX", "UNREAD"))
	e.Gmail.Panic["ModifyThread:"+b.ID+":t2"] = "modify exploded"

	_, err := svc.MarkRead(t.Context(), []dto.ThreadRef{ref(a.ID, "t1"), ref(b.ID, "t2")}, true)
	ae := requireCode(t, apperr.Internal, err)
	assert.Equal(t, []map[string]string{{"accountId": b.ID, "threadId": "t2"}}, failedRefs(t, ae))
	row, err := e.Store.GetThread(t.Context(), a.ID, "t1")
	require.NoError(t, err)
	assert.False(t, row.IsUnread, "panic しなかったアカウントは反映される")
}

func TestArchivePartialFailure(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	e.PutThread(t, a.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX"))
	e.PutThread(t, b.ID, "t3", apptest.Msg("m3", "t3", at(3), "INBOX"))
	e.Gmail.Err["ModifyThread:"+b.ID+":t3"] = gmail.ErrRateLimited

	out, err := svc.Archive(t.Context(), []dto.ThreadRef{ref(a.ID, "t1"), ref(b.ID, "t3"), ref(a.ID, "t2")})
	assert.Nil(t, out)
	ae := requireCode(t, apperr.RateLimited, err)
	assert.Equal(t, []map[string]string{{"accountId": b.ID, "threadId": "t3"}}, failedRefs(t, ae))

	for _, id := range []string{"t1", "t2"} {
		row, gerr := e.Store.GetThread(t.Context(), a.ID, id)
		require.NoError(t, gerr)
		assert.False(t, row.InInbox, "成功分は反映済み")
	}
	row, err := e.Store.GetThread(t.Context(), b.ID, "t3")
	require.NoError(t, err)
	assert.True(t, row.InInbox)
}

func TestMarkReadNeedsReauthMarksAccount(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD"))
	e.Gmail.Err["ModifyThread"] = gmail.ErrNeedsReauth
	_, err := svc.MarkRead(t.Context(), []dto.ThreadRef{ref(a.ID, "t1")}, true)
	_ = requireCode(t, apperr.NeedsReauth, err)
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
}

func TestOpsVanishedThreadIsNotFoundAndRemovedFromDB(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	e.PutThread(t, a.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX"))
	delete(e.Gmail.Threads, apptest.Key(a.ID, "t1")) // Gmail 側で消えた（ModifyThread が 404）

	out, err := svc.Archive(t.Context(), []dto.ThreadRef{ref(a.ID, "t1"), ref(a.ID, "t2")})
	assert.Nil(t, out)
	ae := requireCode(t, apperr.NotFound, err)
	assert.Equal(t, []map[string]string{{"accountId": a.ID, "threadId": "t1"}}, failedRefs(t, ae))
	assert.Equal(t, "スレッドが見つかりません。", ae.Message)
	_, err = e.Store.GetThread(t.Context(), a.ID, "t1")
	assert.ErrorIs(t, err, store.ErrNotFound, "リストから除去する")
	row, err := e.Store.GetThread(t.Context(), a.ID, "t2")
	require.NoError(t, err)
	assert.False(t, row.InInbox, "成功分は反映済み")
}

func TestOpsPrefersNonNotFoundCode(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	e.PutThread(t, a.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX"))
	delete(e.Gmail.Threads, apptest.Key(a.ID, "t1"))
	e.Gmail.Err["ModifyThread:"+a.ID+":t2"] = gmail.ErrRateLimited
	_, err := svc.Archive(t.Context(), []dto.ThreadRef{ref(a.ID, "t1"), ref(a.ID, "t2")})
	ae := requireCode(t, apperr.RateLimited, err)
	assert.Len(t, failedRefs(t, ae), 2)
}

func TestOpsInvalidateBodyCache(t *testing.T) {
	e := apptest.NewEnv(t)
	svc, threads := app.NewInboxService(e.Deps), app.NewThreadService(e.Deps)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD"))
	e.Gmail.Labels[a.ID] = []gmail.Label{{ID: "Label_1", Name: "Work", Type: "user"}}
	ctx := t.Context()
	loads := func() int { return e.Gmail.CallCount("GetThreadFull") }

	_, err := threads.GetThread(ctx, ref(a.ID, "t1"))
	require.NoError(t, err)
	_, err = threads.GetThread(ctx, ref(a.ID, "t1"))
	require.NoError(t, err)
	require.Equal(t, 1, loads(), "2 回目はキャッシュ")

	steps := map[string]func() error{
		"MarkRead": func() error { _, err := svc.MarkRead(ctx, []dto.ThreadRef{ref(a.ID, "t1")}, true); return err },
		"Archive":  func() error { _, err := svc.Archive(ctx, []dto.ThreadRef{ref(a.ID, "t1")}); return err },
		"Labels": func() error {
			_, err := svc.ModifyLabels(ctx, ref(a.ID, "t1"), []string{"Label_1"}, nil)
			return err
		},
		"Trash": func() error { _, err := svc.Trash(ctx, []dto.ThreadRef{ref(a.ID, "t1")}); return err },
	}
	for _, name := range []string{"MarkRead", "Archive", "Labels", "Trash"} {
		before := loads()
		require.NoError(t, steps[name](), name)
		_, err = threads.GetThread(ctx, ref(a.ID, "t1"))
		require.NoError(t, err)
		assert.Equal(t, before+1, loads(), "%s の後の GetThread は再取得される", name)
	}
	d, err := threads.GetThread(ctx, ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.False(t, d.Messages[0].IsUnread, "操作後の状態が見える")
}

func TestTrash(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	out, err := svc.Trash(t.Context(), []dto.ThreadRef{ref(a.ID, "t1")})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.False(t, out[0].InInbox)
	assert.Equal(t, []string{apptest.Key(a.ID, "t1")}, e.Gmail.Trashed)

	_, err = svc.Trash(t.Context(), []dto.ThreadRef{ref("nope", "t1")})
	ae := requireCode(t, apperr.NotFound, err)
	assert.Len(t, failedRefs(t, ae), 1)
}

func TestListLabelsAndModifyLabels(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.Gmail.Labels[a.ID] = []gmail.Label{
		{ID: "INBOX", Name: "INBOX", Type: "system"},
		{ID: "Label_1", Name: "Work", Type: "user", ColorBg: "#111", ColorFg: "#fff"},
		{ID: "Label_2", Name: "Home", Type: "user"},
	}
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))

	labels, err := svc.ListLabels(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, []dto.Label{{ID: "Label_1", Name: "Work", ColorBg: "#111", ColorFg: "#fff"}, {ID: "Label_2", Name: "Home"}}, sortLabels(labels))

	sum, err := svc.ModifyLabels(t.Context(), ref(a.ID, "t1"), []string{"Label_1"}, nil)
	require.NoError(t, err)
	require.Len(t, sum.Labels, 1)
	assert.Equal(t, "Work", sum.Labels[0].Name)
	assert.Equal(t, []string{"Label_1"}, e.Gmail.Modified[0].Add)
	sum, err = svc.ModifyLabels(t.Context(), ref(a.ID, "t1"), nil, []string{"Label_1"})
	require.NoError(t, err)
	assert.Empty(t, sum.Labels)

	for _, bad := range [][2][]string{{{"INBOX"}, nil}, {nil, {"UNREAD"}}, {{"Label_404"}, nil}, {nil, nil}} {
		_, err = svc.ModifyLabels(t.Context(), ref(a.ID, "t1"), bad[0], bad[1])
		_ = requireCode(t, apperr.Validation, err)
	}
	assert.Len(t, e.Gmail.Modified, 2, "検証エラーでは Gmail を呼ばない")

	_, err = svc.ListLabels(t.Context(), "nope")
	_ = requireCode(t, apperr.NotFound, err)
}

func TestListLabelsOfflineUsesCache(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	require.NoError(t, e.Store.ReplaceLabels(t.Context(), a.ID, []store.Label{{ID: "L1", Name: "Cached", Type: "user"}}))
	e.Gmail.Err["ListLabels"] = gmail.ErrOffline
	labels, err := svc.ListLabels(t.Context(), a.ID)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, "Cached", labels[0].Name)
	e.Gmail.Err["ListLabels"] = gmail.ErrRateLimited
	_, err = svc.ListLabels(t.Context(), a.ID)
	_ = requireCode(t, apperr.RateLimited, err)
}

func sortLabels(l []dto.Label) []dto.Label {
	slices.SortFunc(l, func(a, b dto.Label) int { return cmp.Compare(b.Name, a.Name) })
	return l
}

func TestModifyLabelsVanishedThread(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.Gmail.Labels[a.ID] = []gmail.Label{{ID: "Label_1", Name: "Work", Type: "user"}}
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	delete(e.Gmail.Threads, apptest.Key(a.ID, "t1"))
	_, err := svc.ModifyLabels(t.Context(), ref(a.ID, "t1"), []string{"Label_1"}, nil)
	ae := requireCode(t, apperr.NotFound, err)
	assert.Equal(t, a.ID, ae.AccountID)
	_, err = e.Store.GetThread(t.Context(), a.ID, "t1")
	assert.ErrorIs(t, err, store.ErrNotFound)
}
