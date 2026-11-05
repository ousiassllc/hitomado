// Package undotests はアーカイブ・ゴミ箱の取り消し（Unarchive・Untrash）を、実 SQLite・フェイク（apptest）で検証する外部テスト。
package undotests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

func newInboxSvc(t *testing.T) (*app.InboxService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewInboxService(e.Deps), e
}

func at(minutes int) time.Time { return apptest.Epoch.Add(time.Duration(minutes) * time.Minute) }

func ref(acc, thr string) dto.ThreadRef { return dto.ThreadRef{AccountID: acc, ThreadID: thr} }

func requireCode(t *testing.T, want string, err error) *apperr.Error {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, want, ae.Code)
	return ae
}

func failedRefs(t *testing.T, ae *apperr.Error) []map[string]string {
	t.Helper()
	v, ok := ae.Details["failedRefs"].([]map[string]string)
	require.True(t, ok, "details.failedRefs がある: %#v", ae.Details)
	return v
}

func TestUnarchive(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	ctx := t.Context()
	_, err := svc.Archive(ctx, []dto.ThreadRef{ref(a.ID, "t1")})
	require.NoError(t, err)

	out, err := svc.Unarchive(ctx, []dto.ThreadRef{ref(a.ID, "t1")})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.True(t, out[0].InInbox)
	row, err := e.Store.GetThread(ctx, a.ID, "t1")
	require.NoError(t, err)
	assert.True(t, row.InInbox, "DB も受信箱に戻る")
	assert.Equal(t, []string{"INBOX"}, e.Gmail.Modified[len(e.Gmail.Modified)-1].Add)

	_, err = svc.Unarchive(ctx, []dto.ThreadRef{ref("nope", "t1")})
	ae := requireCode(t, apperr.NotFound, err)
	assert.Len(t, failedRefs(t, ae), 1)
}

func TestUntrash(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	e.PutThread(t, a.ID, "t2", apptest.Msg("m2", "t2", at(2)))
	ctx := t.Context()
	_, err := svc.Trash(ctx, []dto.ThreadRef{ref(a.ID, "t1"), ref(a.ID, "t2")})
	require.NoError(t, err)

	out, err := svc.Untrash(ctx, []dto.ThreadRef{ref(a.ID, "t1")}, true)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.True(t, out[0].InInbox, "受信箱にあったスレッドは INBOX に戻る")

	out, err = svc.Untrash(ctx, []dto.ThreadRef{ref(a.ID, "t2")}, false)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.False(t, out[0].InInbox, "受信箱になかったスレッドは INBOX に入れない")
	assert.Equal(t, []string{apptest.Key(a.ID, "t1"), apptest.Key(a.ID, "t2")}, e.Gmail.Untrashed)

	_, err = svc.Untrash(ctx, []dto.ThreadRef{ref("nope", "t1")}, true)
	ae := requireCode(t, apperr.NotFound, err)
	assert.Len(t, failedRefs(t, ae), 1)
}
