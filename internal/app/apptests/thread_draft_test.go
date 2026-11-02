package apptests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

func TestGetThreadSkipsDraftSearchWithoutDraftMessage(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))

	d, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.Empty(t, d.DraftID)
	assert.Zero(t, e.Gmail.CallCount("FindDraftForThread"), "DRAFT メッセージが無ければ drafts.list を走査しない")
}

func TestGetThreadWithDraftMessageSearchesDraft(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"), apptest.Msg("m-draft", "t1", at(2), "DRAFT"))
	e.Gmail.Drafts[apptest.Key(a.ID, "d1")] = gmail.DraftFull{ID: "d1", ThreadID: "t1"}

	d, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.Equal(t, "d1", d.DraftID)
	assert.Equal(t, 1, e.Gmail.CallCount("FindDraftForThread"))
}

// 下書きのないスレッドを開いてキャッシュされた後でも、SaveDraft 直後に開くと draftId が返り、
// DiscardCompose 後は返らない（SaveDraft／DiscardCompose がスレッド本文のキャッシュを更新する）。
func TestGetThreadReflectsSaveDraftAndDiscard(t *testing.T) {
	e := apptest.NewEnv(t)
	threads, compose := app.NewThreadService(e.Deps), app.NewComposeService(e.Deps)
	a := e.AddAccount(t, "me@a.example")
	m := apptest.Msg("m1", "t1", at(1), "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.To = []model.Address{{Email: "me@a.example"}}
	e.PutThread(t, a.ID, "t1", m)

	d, err := threads.GetThread(t.Context(), ref(a.ID, "t1")) // 下書きなしの状態をキャッシュさせる
	require.NoError(t, err)
	require.Empty(t, d.DraftID)

	s, err := compose.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &dto.ThreadRef{AccountID: a.ID, ThreadID: "t1"}})
	require.NoError(t, err)
	saved, err := compose.SaveDraft(t.Context(), s.SessionID, s.Content)
	require.NoError(t, err)
	require.NotEmpty(t, saved.DraftID)

	d, err = threads.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.Equal(t, saved.DraftID, d.DraftID, "SaveDraft 直後に開くと下書きの導線が出る")
	assert.Len(t, d.Messages, 1, "下書きは表示しない")

	require.NoError(t, compose.DiscardCompose(t.Context(), s.SessionID))
	d, err = threads.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.Empty(t, d.DraftID, "破棄した下書きは返さない")
}
