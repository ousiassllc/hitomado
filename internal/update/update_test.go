package update_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/update"
)

func TestIsDev(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{update.DevVersion, true},
		{"0.0.0", true},
		{"0.0.1", false},
		{"0.1.0", false},
		{"0.99.2", false},
		{"1.2.3", false},
		{"", false},
	} {
		assert.Equal(t, tc.want, update.IsDev(tc.version), "IsDev(%q)", tc.version)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "0.1.1", true},
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "1.0.0", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.1", "0.1.0", false},
		{"1.0.0", "0.9.9", false},
		{"0.9.0", "0.10.0", true}, // 辞書順でなく数値比較
		{"0.10.0", "0.9.0", false},
		{"v0.1.0", "v0.1.1", true},
		{"0.1.0", "v0.1.1", true},
		{"V0.1.0", "0.1.0", false},
		{"0.1.0-beta.1", "0.1.0", true}, // プレリリースは正式版より古い
		{"0.1.0", "0.1.0-beta.1", false},
		{"0.1.0-beta.1", "0.1.0-beta.2", true},
		{"0.1.0-beta.2", "0.1.0-beta.10", true},
		{"0.1.0-alpha", "0.1.0-beta", true},
		{"0.1.0-alpha", "0.1.0-alpha.1", true},
		{"0.1.0-alpha.1", "0.1.0-alpha", false},
		{"0.1.0-1", "0.1.0-alpha", true}, // 数字のみ < 英字
		{"0.1.0-rc.1", "0.1.1", true},
		{"0.1.0+build5", "0.1.0", false}, // ビルドメタデータは無視
		{"0.1.0", "0.1.1+build5", true},
		{"1.2", "1.2.1", true},
		{"1", "1.0.1", true},
		{"1.2", "1.2.0", false},
		{"dev", "0.1.0", false}, // 解析不能は false
		{"0.1.0", "dev", false},
		{"", "0.1.0", false},
		{"0.1.0", "", false},
		{"0.1.x", "0.1.1", false},
		{"0.1.0.1", "0.1.1", false},
		{"0.1.0-", "0.1.1", false},
		{"0.-1.0", "0.1.1", false},
		{"0.1.0", "99999999999999999999.0.0", false}, // オーバーフローは解析不能
		{" 0.1.0 ", "0.1.1", true},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, update.IsNewer(c.current, c.latest), "IsNewer(%q, %q)", c.current, c.latest)
	}
}

func TestLatest(t *testing.T) {
	var gotPath, gotUA, gotAccept, gotAuth, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA, gotAccept = r.URL.RequestURI(), r.Header.Get("User-Agent"), r.Header.Get("Accept")
		gotAuth, gotCookie = r.Header.Get("Authorization"), r.Header.Get("Cookie")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/ousiassllc/hitomado/releases/tag/v0.2.0","name":"x"}`))
	}))
	t.Cleanup(srv.Close)

	c := update.NewGitHub(update.Options{HTTPClient: srv.Client(), BaseURL: srv.URL + "/", UserAgent: "Hitomado/0.1.0"})
	rel, err := c.Latest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.2.0", rel.Version)
	assert.Equal(t, "https://github.com/ousiassllc/hitomado/releases/tag/v0.2.0", rel.URL)
	assert.Equal(t, "/repos/ousiassllc/hitomado/releases/latest", gotPath)
	assert.Equal(t, "Hitomado/0.1.0", gotUA)
	assert.Contains(t, gotAccept, "application/vnd.github")
	assert.Empty(t, gotAuth, "未認証")
	assert.Empty(t, gotCookie)
}

// 配布物がインストーラー（hitomado-vX.Y.Z-windows-amd64-setup.exe）と SHA256SUMS.txt になっても、
// 更新確認はタグとリリースページだけで判定する（配布物名に依存しない）。
func TestLatestIgnoresAssetNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.2","html_url":"https://github.com/ousiassllc/hitomado/releases/tag/v0.1.2",
			"assets":[
				{"name":"hitomado-v0.1.2-windows-amd64-setup.exe","browser_download_url":"https://example.invalid/setup.exe"},
				{"name":"SHA256SUMS.txt","browser_download_url":"https://example.invalid/SHA256SUMS.txt"}]}`))
	}))
	t.Cleanup(srv.Close)

	rel, err := update.NewGitHub(update.Options{HTTPClient: srv.Client(), BaseURL: srv.URL}).Latest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.1.2", rel.Version)
	assert.Equal(t, "https://github.com/ousiassllc/hitomado/releases/tag/v0.1.2", rel.URL)
	assert.True(t, update.IsNewer("0.1.1", rel.Version), "インストーラーが入れた版（0.1.1）より新しい")
	assert.False(t, update.IsNewer("0.1.2", rel.Version), "同じ版では通知しない")
}

func TestLatestCustomRepoAndTagWithoutV(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"tag_name":"1.0.0","html_url":"u"}`))
	}))
	t.Cleanup(srv.Close)

	rel, err := update.NewGitHub(update.Options{HTTPClient: srv.Client(), BaseURL: srv.URL, Repo: "o/r"}).Latest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", rel.Version)
	assert.Equal(t, "/repos/o/r/releases/latest", gotPath)
}

func TestLatestErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"not found", 404, `{"message":"Not Found"}`},
		{"rate limited", 403, `{"message":"rate limit"}`},
		{"server error", 500, ``},
		{"invalid json", 200, `<html>`},
		{"missing tag", 200, `{"html_url":"u"}`},
		{"empty tag", 200, `{"tag_name":"v","html_url":"u"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			t.Cleanup(srv.Close)
			_, err := update.NewGitHub(update.Options{HTTPClient: srv.Client(), BaseURL: srv.URL}).Latest(context.Background())
			require.Error(t, err)
		})
	}
}

func TestLatestNetworkErrorIsNetError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()

	_, err := update.NewGitHub(update.Options{BaseURL: base}).Latest(context.Background())
	var ne net.Error
	require.ErrorAs(t, err, &ne)
}

func TestLatestContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := update.NewGitHub(update.Options{HTTPClient: srv.Client(), BaseURL: srv.URL}).Latest(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
