package agentcoretests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
)

// restart はアプリの再起動に相当する新しいサービス一式（メモリのキャッシュは空。Store・Gmail は同じ）を返す。
func (f *fixture) restart() *app.Services { return app.NewServices(f.Deps) }

// 下書きを作った直後のプレビューは、作った本文からメモリだけで作る（Gmail の下書きを取りに行かない）。
func TestPreviewRightAfterCreateComesFromMemory(t *testing.T) {
	f := newFixture(t)
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)

	it := f.only(t)
	assert.Contains(t, it.DraftPreview, secretDraft)
	assert.Zero(t, f.Gmail.CallCount("GetDraft"), "キャッシュにあれば Gmail に問い合わせない")
}

// 再起動後（キャッシュ無し）は、ready／later／done の行だけ Gmail.GetDraft で作る。署名・引用は含めない。結果は覚えておく。
func TestPreviewAfterRestartIsBuiltFromGmailDraft(t *testing.T) {
	f := newFixture(t)
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	f.stop()

	svc := f.restart()
	o, err := svc.Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	require.Len(t, o.Items, 1)
	assert.Contains(t, o.Items[0].DraftPreview, secretDraft)
	assert.NotContains(t, o.Items[0].DraftPreview, "署名A", "署名は含めない")
	assert.NotContains(t, o.Items[0].DraftPreview, secretBody, "引用は含めない")
	assert.Equal(t, 1, f.Gmail.CallCount("GetDraft"))

	_, err = svc.Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, f.Gmail.CallCount("GetDraft"), "2 回目はメモリから")

	later, err := svc.Agent.DeferAgentItem(t.Context(), o.Items[0].ID)
	require.NoError(t, err)
	assert.Contains(t, later.DraftPreview, secretDraft)
	f.requireNeverSent(t)
}

// 下書きが取れない（NotFound 以外の失敗）ときも一覧は成功し、その行のプレビューだけ空。項目の状態は変えない。ログはアカウント ID とエラーの種類だけ。
func TestListItemsSucceedsWhenGetDraftFails(t *testing.T) {
	for name, tc := range map[string]struct {
		inject func(f *fixture, draftID string)
		kind   string
	}{
		"error": {func(f *fixture, _ string) { f.Gmail.Err["GetDraft"] = errors.New("boom " + secretDraft) }, "internal"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.enable(t)
			f.newMail(f.a.ID, "t1", "m1")
			f.newMail(f.b.ID, "u1", "u-m1")
			f.waitItems(t, 2)
			f.stop()
			var draftID string
			for _, it := range f.items(t).Items {
				if it.AccountID == f.a.ID {
					draftID = it.DraftID
				}
			}
			tc.inject(f, draftID)

			o, err := f.restart().Agent.ListAgentItems(t.Context())
			require.NoError(t, err)
			require.Len(t, o.Items, 2)
			for _, it := range o.Items {
				assert.NotEmpty(t, it.DraftID)
				assert.Empty(t, it.DraftPreview)
				assert.Equal(t, "ready", it.Status, "失敗では項目を倒さない")
			}
			assert.Contains(t, f.logs.String(), "draft preview failed")
			assert.Contains(t, f.logs.String(), f.a.ID)
			assert.Contains(t, f.logs.String(), "kind="+tc.kind)
			assert.Empty(t, containsAny(f.logs.String(), secretDraft, secretBody, secretSubject), "ログに本文・件名を出さない")
		})
	}
}
