package storetest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/store"
)

func TestOpenAppliesMigrationsAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "h.db")

	s1, err := store.Open(ctx, path)
	require.NoError(t, err)
	oc := store.OAuthClient{ID: store.NewID(), Name: "c", ClientID: "cid"}
	require.NoError(t, s1.CreateOAuthClient(ctx, oc))
	require.NoError(t, s1.SetSetting(ctx, store.SettingUpdateCheck, "true"))
	require.NoError(t, s1.Close())

	// 再オープンではマイグレーションが再適用されず、データが残る。
	s2, err := store.Open(ctx, path)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	clients, err := s2.ListOAuthClients(ctx)
	require.NoError(t, err)
	require.Len(t, clients, 1)
	assert.Equal(t, oc.ID, clients[0].ID)
	v, ok, err := s2.GetSetting(ctx, store.SettingUpdateCheck)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "true", v)
}

func TestOpenFailsOnBrokenDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	require.NoError(t, os.WriteFile(path, []byte("this is not a sqlite database file........................"), 0o600))
	_, err := store.Open(context.Background(), path)
	require.Error(t, err)
}

func TestSchemaStoresNoBodyAttachmentOrDraft(t *testing.T) {
	s, _ := newTestStore(t)
	rows, err := s.raw.Query(`SELECT m.name || '.' || p.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var col string
		require.NoError(t, rows.Scan(&col))
		if col == "threads.has_attachment" { // 添付の有無の近似フラグのみ許可（添付データは持たない）
			continue
		}
		// エージェントモード（#372）が作った Gmail の下書きの参照（ID。本文ではない）だけ許可する。
		if col == "agent_items.draft_id" {
			continue
		}
		assert.NotRegexp(t, `(?i)body|attachment|draft`, col)
	}
	require.NoError(t, rows.Err())
}

func TestSettings(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()

	_, ok, err := s.GetSetting(ctx, store.SettingInboxFilter)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.SetSetting(ctx, store.SettingInboxFilter, `{"accountIds":[],"unreadOnly":false}`))
	clk.Advance(1)
	require.NoError(t, s.SetSetting(ctx, store.SettingInboxFilter, `{"accountIds":["a"],"unreadOnly":true}`))
	v, ok, err := s.GetSetting(ctx, store.SettingInboxFilter)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.JSONEq(t, `{"accountIds":["a"],"unreadOnly":true}`, v)
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM app_settings`))

	// json_valid の CHECK で不正な JSON は拒否される。
	require.Error(t, s.SetSetting(ctx, store.SettingAIConfig, `{not json`))
	_, ok, err = s.GetSetting(ctx, store.SettingAIConfig)
	require.NoError(t, err)
	assert.False(t, ok)
}
