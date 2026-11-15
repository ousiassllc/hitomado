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

// 00002_ai_features・00003_ai_thread_feature: 旧 ai.config を「AI で下書き」「要約・質問」の選択（ai.features。以前は同じ設定を共有していたので同じ値）と
// OpenAI 互換の Base URL（ai.connection）へ移し、旧キーを消す（#376）。
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
			features:   `{"draft":{"provider":"openai","model":"gpt-old","effort":"high"},"threadAi":{"provider":"openai","model":"gpt-old","effort":"high"}}`,
			connection: `{"openaiBaseUrl":"https://llm.example/v1"}`,
		},
		{
			name:     "Base URL の無い anthropic",
			legacy:   `{"provider":"anthropic","model":"claude-x"}`,
			features: `{"draft":{"provider":"anthropic","model":"claude-x"},"threadAi":{"provider":"anthropic","model":"claude-x"}}`,
		},
		{
			name:     "chatgpt の旧 Base URL は引き継がない",
			legacy:   `{"provider":"chatgpt","model":"gpt-plan","baseUrl":"https://ignored.example/v1"}`,
			features: `{"draft":{"provider":"chatgpt","model":"gpt-plan"},"threadAi":{"provider":"chatgpt","model":"gpt-plan"}}`,
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

// 00002 を適用済みの DB（draft だけの ai.features）に 00003 を当てると、新規に全部流した DB と同じ結果（threadAi に draft の選択）になる。
// すでに threadAi があれば変えず、Down は threadAi だけを捨てて 00002 の形（draft だけ）へ戻す。
func TestMigrateThreadAIFeatureOnAppliedDB(t *testing.T) {
	cases := []struct {
		name   string
		before string
		after  string
		down   string
	}{
		{
			name:   "draft だけ（00002 適用済み）は threadAi に写す",
			before: `{"draft":{"provider":"anthropic","model":"claude-x","effort":"high"},"inboxZero":{"provider":"typesafe"}}`,
			after:  `{"draft":{"provider":"anthropic","model":"claude-x","effort":"high"},"threadAi":{"provider":"anthropic","model":"claude-x","effort":"high"},"inboxZero":{"provider":"typesafe"}}`,
			down:   `{"draft":{"provider":"anthropic","model":"claude-x","effort":"high"},"inboxZero":{"provider":"typesafe"}}`,
		},
		{
			name:   "threadAi が選ばれていれば変えない",
			before: `{"draft":{"provider":"anthropic","model":"a"},"threadAi":{"provider":"openai","model":"b"}}`,
			after:  `{"draft":{"provider":"anthropic","model":"a"},"threadAi":{"provider":"openai","model":"b"}}`,
			down:   `{"draft":{"provider":"anthropic","model":"a"}}`,
		},
		{
			name:   "draft が無ければ作らない",
			before: `{"inboxZero":{"provider":"typesafe"}}`,
			after:  `{"inboxZero":{"provider":"typesafe"}}`,
			down:   `{"inboxZero":{"provider":"typesafe"}}`,
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

			_, err = p.UpTo(ctx, 2)
			require.NoError(t, err)
			_, err = conn.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('ai.features', ?, 1)`, tc.before)
			require.NoError(t, err)
			get := func() string {
				var v string
				require.NoError(t, conn.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'ai.features'`).Scan(&v))
				return v
			}

			_, err = p.UpByOne(ctx)
			require.NoError(t, err)
			assert.JSONEq(t, tc.after, get())
			_, err = p.Down(ctx)
			require.NoError(t, err)
			assert.JSONEq(t, tc.down, get())
		})
	}
}
