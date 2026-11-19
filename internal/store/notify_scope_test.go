package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/store"
	"github.com/ousiassllc/nefos-inbox/internal/synctest"
)

func TestNotifyScopeAllows(t *testing.T) {
	inboxUnread := []string{"INBOX", "UNREAD"}
	cases := []struct {
		name   string
		labels []string
		want   map[store.NotifyScope]bool
	}{
		{"カテゴリなし（メイン）", inboxUnread, map[store.NotifyScope]bool{store.NotifyScopePrimary: true, store.NotifyScopeAll: true}},
		{"CATEGORY_PERSONAL（メイン）", append([]string{"CATEGORY_PERSONAL"}, inboxUnread...), map[store.NotifyScope]bool{store.NotifyScopePrimary: true, store.NotifyScopeAll: true}},
		{"ソーシャル", append([]string{"CATEGORY_SOCIAL"}, inboxUnread...), map[store.NotifyScope]bool{store.NotifyScopePrimary: false, store.NotifyScopeAll: true}},
		{"プロモーション", append([]string{"CATEGORY_PROMOTIONS"}, inboxUnread...), map[store.NotifyScope]bool{store.NotifyScopePrimary: false, store.NotifyScopeAll: true}},
		{"新着情報", append([]string{"CATEGORY_UPDATES"}, inboxUnread...), map[store.NotifyScope]bool{store.NotifyScopePrimary: false, store.NotifyScopeAll: true}},
		{"フォーラム", append([]string{"CATEGORY_FORUMS"}, inboxUnread...), map[store.NotifyScope]bool{store.NotifyScopePrimary: false, store.NotifyScopeAll: true}},
	}
	for _, c := range cases {
		for scope, want := range c.want {
			assert.Equal(t, want, scope.Allows(c.labels), "%s / %s", c.name, scope)
		}
	}
}

func TestLoadNotifyScopeDefaultsToPrimary(t *testing.T) {
	ctx := context.Background()
	for name, raw := range map[string]*string{
		"未設定":  nil,
		"壊れた値": ptr(`{"x":1}`),
		"未知の値": ptr(`"everything"`),
		"空文字":  ptr(`""`),
	} {
		st := synctest.NewStore()
		if raw != nil {
			require.NoError(t, st.SetSetting(ctx, store.SettingNotifyScope, *raw))
		}
		got, err := store.LoadNotifyScope(ctx, st)
		require.NoError(t, err, name)
		assert.Equal(t, store.NotifyScopePrimary, got, name)
	}
}

func TestNotifyScopeRoundTripAndRejectsUnknown(t *testing.T) {
	ctx := context.Background()
	st := synctest.NewStore()
	require.NoError(t, store.SaveNotifyScope(ctx, st, store.NotifyScopeAll))
	got, err := store.LoadNotifyScope(ctx, st)
	require.NoError(t, err)
	assert.Equal(t, store.NotifyScopeAll, got)

	require.Error(t, store.SaveNotifyScope(ctx, st, "nope"))
	got, _ = store.LoadNotifyScope(ctx, st)
	assert.Equal(t, store.NotifyScopeAll, got, "未知の値は保存しない")
}

func ptr(s string) *string { return &s }
