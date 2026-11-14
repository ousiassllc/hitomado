package agentcoretests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// 同じ新着（アカウント・メッセージ）は、何度来ても 1 度しか扱わない。
func TestSameNewMailIsHandledOnce(t *testing.T) {
	f := newFixture(t)
	f.enable(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	f.newMail(f.a.ID, "t1", "m1")
	f.drain(t)

	judges, generates := f.ai.counts()
	assert.Equal(t, 2, judges, "t1 の 1 回と番兵")
	assert.Equal(t, 2, generates)
	assert.Equal(t, 2, f.Gmail.CallCount("CreateDraft"))
	assert.Len(t, f.items(t).Items, 2)
}

// スレッドにすでに自分の下書きがある・受信箱から外れた・消えたスレッドは扱わない。
func TestSkipsThreadsWithUserDraftOrGone(t *testing.T) {
	f := newFixture(t)
	f.enable(t)
	draft := apptest.Msg("d1", "t1", apptest.Epoch, "DRAFT")
	draft.From = model.Address{Email: f.a.Email}
	m1 := apptest.Msg("m1", "t1", apptest.Epoch, "INBOX", "UNREAD")
	f.PutThread(t, f.a.ID, "t1", m1, draft)

	f.newMail(f.a.ID, "t1", "m1")
	f.newMail(f.a.ID, "no-such-thread", "mx")
	f.newMail(f.a.ID, "t1", "m1")
	f.drain(t)

	judges, _ := f.ai.counts()
	assert.Equal(t, 1, judges, "番兵だけ")
	require.Len(t, f.items(t).Items, 1)
	assert.Equal(t, f.b.ID, f.only(t).AccountID, "番兵以外の項目は無い")
}
