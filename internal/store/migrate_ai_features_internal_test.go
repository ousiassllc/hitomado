package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 00002_ai_features: 旧 ai.config を「AI で下書き」の選択（ai.features）と OpenAI 互換の Base URL（ai.connection）へ移し、旧キーを消す（#376）。
func TestMigrateAIConfigToFeatures(t *testing.T) {
	cases := []struct {
		name       string
		legacy     string
		features   string
		connection string // 空はキー無し
	}{
		{
			name:       "openai は Base URL をつなぎ先へ移す",
			legacy:     `{"provider":"openai","model":"gpt-old","baseUrl":"https://llm.example/v1","effort":"high"}`,
			features:   `{"draft":{"provider":"openai","model":"gpt-old","effort":"high"}}`,
			connection: `{"openaiBaseUrl":"https://llm.example/v1"}`,
		},
		{
			name:     "Base URL の無い anthropic",
			legacy:   `{"provider":"anthropic","model":"claude-x"}`,
			features: `{"draft":{"provider":"anthropic","model":"claude-x"}}`,
		},
		{
			name:     "chatgpt の旧 Base URL は引き継がない",
			legacy:   `{"provider":"chatgpt","model":"gpt-plan","baseUrl":"https://ignored.example/v1"}`,
			features: `{"draft":{"provider":"chatgpt","model":"gpt-plan"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			conn, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "h.db")))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			sub, err := fs.Sub(migrationsFS, "migrations")
			require.NoError(t, err)
			p, err := goose.NewProvider(goose.DialectSQLite3, conn, sub)
			require.NoError(t, err)

			_, err = p.UpTo(ctx, 1)
			require.NoError(t, err)
			_, err = conn.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('ai.config', ?, 1)`, tc.legacy)
			require.NoError(t, err)
			_, err = conn.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('ai.consent', '{"providers":[]}', 1)`)
			require.NoError(t, err)

			_, err = p.Up(ctx)
			require.NoError(t, err)

			get := func(key string) (string, bool) {
				var v string
				err := conn.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&v)
				if err == sql.ErrNoRows {
					return "", false
				}
				require.NoError(t, err)
				return v, true
			}
			v, ok := get("ai.features")
			require.True(t, ok)
			assert.JSONEq(t, tc.features, v)
			v, ok = get("ai.connection")
			assert.Equal(t, tc.connection != "", ok)
			if ok {
				assert.JSONEq(t, tc.connection, v)
			}
			_, ok = get("ai.config")
			assert.False(t, ok, "旧キーは残さない")
			_, ok = get("ai.consent")
			assert.True(t, ok, "同意は変えない")
		})
	}
}

// 旧 ai.config が無い（AI を一度も設定していない）なら、何も作らない。
func TestMigrateWithoutLegacyAIConfigCreatesNothing(t *testing.T) {
	ctx := context.Background()
	s, err := open(ctx, filepath.Join(t.TempDir(), "h.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	for _, k := range []string{SettingAIFeatures, SettingAIConnection} {
		_, ok, err := s.GetSetting(ctx, k)
		require.NoError(t, err)
		assert.False(t, ok, k)
	}
}
