package agentcoretests

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/agentcore"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 既定はすべてオフ。AI が未設定なら aiReady も偽。
func TestSettingsDefaultOff(t *testing.T) {
	f := newFixture(t)
	got, err := f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.False(t, got.MascotEnabled)
	assert.False(t, got.AIReady)

	f.configure(t)
	got, err = f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.False(t, got.AIReady, "設定済みでも、同意がなければ使えない状態")
	f.ready(t)
	got, err = f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.True(t, got.AIReady)
	assert.False(t, got.Enabled, "AI が使えても、オンにするまではオフ")
}

// オンにするには同意（agreed）が要る。同意が無ければ何も変えない。
func TestEnableRequiresAgreement(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, false)
	requireCode(t, "VALIDATION", err)
	got, err := f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Empty(t, f.Events.Named(events.AgentUpdated), "変わっていないので通知しない")
}

// AI が未設定なら AI_NOT_CONFIGURED（フロントが AI の設定へ案内する）。同意は記録しない。
func TestEnableWithoutAIConfiguredFails(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	requireCode(t, "AI_NOT_CONFIGURED", err)

	cfg, err := f.svc.AI.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsentedProviders)
	got, err := f.svc.Agent.GetAgentSettings(t.Context())
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}

// 設定済み・未同意でオンにすると、現在のプロバイダ・送信先への同意を記録して有効にする。オフにするのに同意は要らない。
func TestEnableRecordsConsentAndDisableNeedsNone(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	cfg, err := f.svc.AI.GetAIConfig(t.Context())
	require.NoError(t, err)
	require.Empty(t, cfg.ConsentedProviders)

	got, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.True(t, got.AIReady)
	cfg, err = f.svc.AI.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders, "AI の同意（RecordAIConsent と同じ保存）が記録される")

	v, ok, err := f.Store.GetSetting(t.Context(), store.SettingAgent)
	require.NoError(t, err)
	require.True(t, ok)
	var stored map[string]any
	require.NoError(t, json.Unmarshal([]byte(v), &stored))
	assert.Equal(t, true, stored["enabled"])
	assert.NotZero(t, stored["agreedAt"])

	got, err = f.svc.Agent.SetAgentEnabled(t.Context(), false, false)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}

// 設定を変えるたびに agent:updated を発行し、マスコットはエージェントとマスコットが両方オンのときだけ出す。
func TestSettingsEmitEventsAndDriveMascot(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	ctx := t.Context()

	_, err := f.svc.Agent.SetAgentMascot(ctx, true)
	require.NoError(t, err)
	_, err = f.svc.Agent.SetAgentEnabled(ctx, true, true)
	require.NoError(t, err)
	_, err = f.svc.Agent.SetAgentEnabled(ctx, false, false)
	require.NoError(t, err)
	_, err = f.svc.Agent.SetAgentEnabled(ctx, true, true)
	require.NoError(t, err)
	got, err := f.svc.Agent.SetAgentMascot(ctx, false)
	require.NoError(t, err)

	assert.True(t, got.Enabled)
	assert.False(t, got.MascotEnabled)
	assert.Equal(t, []bool{false, true, false, true, false}, f.mascot.got(), "マスコット単独・エージェント単独では出さない")
	ev := f.Events.Named(events.AgentUpdated)
	require.Len(t, ev, 5)
	last, ok := ev[4].Payload.(events.AgentUpdatedPayload)
	require.True(t, ok)
	assert.Equal(t, events.AgentUpdatedPayload{PendingCount: 0, Enabled: true, MascotEnabled: false}, last)
}

// Start は保存済みの設定でマスコットの表示を反映する。Mascot が無い OS（nil）では何もしない。
func TestStartAppliesMascotVisibility(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAgent, `{"enabled":true,"mascotEnabled":true}`))
	f.start(t)
	assert.Equal(t, []bool{true}, f.mascot.got())

	g := newFixture(t)
	g.Deps.Mascot = nil
	g.svc = app.NewServices(g.Deps)
	g.ready(t)
	_, err := g.svc.Agent.SetAgentMascot(t.Context(), true)
	require.NoError(t, err)
	g.start(t)
}

// Start のときに、30 日より古い done／skipped／failed を削除する（確認待ち・質問待ちは残す）。
func TestStartPurgesOldFinishedItems(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	create := func(id, status string) {
		_, err := f.Store.CreateAgentItem(ctx, store.AgentItem{AccountID: f.a.ID, ThreadID: "t", MessageID: id, Status: status, Subject: "s", FromLabel: "f", ReasonKind: "other", Reason: "r"})
		require.NoError(t, err)
	}
	create("old-done", store.AgentStatusDone)
	create("old-ready", store.AgentStatusReady)
	f.Clock.Advance(31 * 24 * time.Hour)
	create("new-done", store.AgentStatusDone)

	f.start(t)
	items := f.items(t).Items
	require.Len(t, items, 2)
	statuses := []string{items[0].Status, items[1].Status}
	assert.ElementsMatch(t, []string{"ready", "done"}, statuses)
}

// OpenPanel はメインウィンドウを前面に出し、パネルを開かせる。ShowMainWindow が無くてもイベントは出す。
func TestOpenPanel(t *testing.T) {
	f := newFixture(t)
	shown := 0
	f.Deps.ShowMainWindow = func() { shown++ }
	svc := app.NewServices(f.Deps)
	require.NoError(t, svc.Agent.OpenAgentPanel(t.Context()))
	assert.Equal(t, 1, shown)
	assert.Len(t, f.Events.Named(events.AgentOpenPanel), 1)

	f.Deps.ShowMainWindow = nil
	svc = app.NewServices(f.Deps)
	require.NoError(t, svc.Agent.OpenAgentPanel(t.Context()))
	assert.Len(t, f.Events.Named(events.AgentOpenPanel), 2)
}

// エージェントの依存（型）には送信のメソッドが無い。送信の経路をエージェントのコードから呼べない構造を保つ。
func TestAgentDependenciesCannotSend(t *testing.T) {
	deps := reflect.TypeFor[agentcore.Deps]()
	for i := range deps.NumField() {
		ft := deps.Field(i).Type
		if ft.Kind() != reflect.Interface {
			continue
		}
		for j := range ft.NumMethod() {
			name := ft.Method(j).Name
			assert.False(t, strings.Contains(strings.ToLower(name), "send"), "%s.%s は送信の経路になり得る", deps.Field(i).Name, name)
		}
	}
}
