package agentcoretests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// AI の状態を確かめられなくても（保存された設定が壊れているなど）、設定の取得・一覧は失敗しない（起動時のアラートを出さない）。
// aiReady は偽で、ログはエラーの種類だけ（アカウント ID なし）。
func TestSettingsAndListDoNotFailWhenAIStateUnreadable(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIConfig, `"broken"`))

	got, err := f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.False(t, got.AIReady)
	assert.False(t, got.Enabled)
	o, err := f.svc.Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	assert.Empty(t, o.Items)
	assert.Contains(t, f.logs.String(), "ai readiness check failed")
	assert.Contains(t, f.logs.String(), "kind=internal")
	assert.NotContains(t, f.logs.String(), f.a.ID)
}

// 同意はオンにしたときのプロバイダ＋送信先に紐づく。あとで別の送信先へ切り替えたら（その送信先に同意済みでも）、
// 新着で AI を呼ばず、aiReady は偽。オフ→オンで保存し直すと、また動く。
func TestSwitchingDestinationStopsTheAgentUntilReEnabled(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	ctx := t.Context()
	_, err := f.svc.AI.RecordAIConsent(ctx, "openai", "https://other.example/v1") // 別の送信先にも同意済み
	require.NoError(t, err)
	_, err = f.svc.Agent.SetAgentEnabled(ctx, true, true)
	require.NoError(t, err)
	f.start(t)
	raw, ok, err := f.Store.GetSetting(ctx, store.SettingAgent)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, raw, "agreedDestination")

	_, err = f.svc.AI.SetAIConfig(ctx, "openai", "gpt-test", "https://other.example/v1", "")
	require.NoError(t, err)
	got, err := f.svc.Agent.GetAgentSettings(ctx)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.False(t, got.AIReady, "オンにしたときと違う送信先なので、使えない状態として示す")

	f.newMail(f.a.ID, "t1", "m1")
	f.waitLog(t, "agent: skipped", 1)
	judges, _ := f.ai.counts()
	assert.Zero(t, judges, "AI を呼ばない")

	got, err = f.svc.Agent.SetAgentEnabled(ctx, false, false)
	require.NoError(t, err)
	got, err = f.svc.Agent.SetAgentEnabled(ctx, true, true)
	require.NoError(t, err)
	assert.True(t, got.AIReady)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	judges, _ = f.ai.counts()
	assert.Equal(t, 1, judges)
}

// この項目が無かった頃の保存値（agreedDestination なし）は、今の設定を受け入れる。
func TestLegacyStoredSettingsAcceptCurrentDestination(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAgent, `{"enabled":true,"agreedAt":1}`))
	f.start(t)

	got, err := f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.True(t, got.AIReady)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
}

// 同期の上限で渡されなかった候補の件数は、オンのときだけログに出す。オフのときは何も出さない。
func TestDroppedCandidatesAreLoggedOnlyWhileOn(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	cands := []syncer.NewMailCandidate{}

	f.svc.Agent.OnNewMail(context.Background(), f.a.ID, cands, 3)
	assert.NotContains(t, f.logs.String(), "too many new mail candidates", "オフでは何も出さない")

	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	require.NoError(t, err)
	f.svc.Agent.OnNewMail(context.Background(), f.a.ID, cands, 3)
	assert.Contains(t, f.logs.String(), "too many new mail candidates")
	assert.Contains(t, f.logs.String(), "count=3")
}
