package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/update"
	"github.com/ousiassllc/hitomado/internal/update/updatetest"
)

var installerBody = []byte("MZ dummy installer body")

func latest(t *testing.T, srv *updatetest.Server) update.Release {
	t.Helper()
	rel, err := srv.Checker().Latest(t.Context())
	require.NoError(t, err)
	return rel
}

func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestStageDownloadsAndVerifies(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	dir := filepath.Join(t.TempDir(), "updates")

	got, err := srv.Stager(dir).Stage(t.Context(), latest(t, srv))
	require.NoError(t, err)

	assert.Equal(t, "0.2.0", got.Version)
	assert.Equal(t, filepath.Join(dir, "hitomado-v0.2.0-windows-amd64-setup.exe"), got.Path)
	assert.Equal(t, updatetest.Sum(installerBody), got.SHA256)
	body, err := os.ReadFile(got.Path)
	require.NoError(t, err)
	assert.Equal(t, installerBody, body)
	assert.Equal(t, []string{"hitomado-v0.2.0-windows-amd64-setup.exe"}, dirFiles(t, dir), "一時ファイルが残らない")
}

func TestStageFollowsRedirectToAllowedHost(t *testing.T) {
	cdn := updatetest.NewServer(t, "0.2.0", installerBody) // 配信元（release-assets.githubusercontent.com の代わり）
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	srv.RedirectTo = cdn.URL
	dir := t.TempDir()

	_, err := srv.Stager(dir).Stage(t.Context(), latest(t, srv))
	require.NoError(t, err)
	assert.Equal(t, 1, cdn.Hits("/dl/hitomado-v0.2.0-windows-amd64-setup.exe"))
}

func TestStageRejectsChecksumMismatch(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	srv.Sums = updatetest.Sum([]byte("something else")) + "  hitomado-v0.2.0-windows-amd64-setup.exe\n"
	dir := t.TempDir()

	_, err := srv.Stager(dir).Stage(t.Context(), latest(t, srv))
	require.ErrorIs(t, err, update.ErrChecksumMismatch)
	assert.Empty(t, dirFiles(t, dir), "合わなければ使わずに捨てる")
}

func TestStageChecksumFileForms(t *testing.T) {
	sum := updatetest.Sum(installerBody)
	name := "hitomado-v0.2.0-windows-amd64-setup.exe"
	for label, sums := range map[string]string{
		"他のファイルの行の後（NOTICE を含む）": strings.Repeat("0", 64) + "  NOTICE\n" + strings.ToUpper(sum) + "  " + name + "\n",
		"バイナリ指定の *":              sum + " *" + name + "\r\n",
	} {
		t.Run(label, func(t *testing.T) {
			srv := updatetest.NewServer(t, "0.2.0", installerBody)
			srv.Sums = sums
			_, err := srv.Stager(t.TempDir()).Stage(t.Context(), latest(t, srv))
			require.NoError(t, err)
		})
	}
	for label, sums := range map[string]string{
		"該当の行が無い":       strings.Repeat("0", 64) + "  NOTICE\n",
		"ハッシュが 64 桁でない": "abc  " + name + "\n",
		"空":             "",
	} {
		t.Run(label, func(t *testing.T) {
			srv := updatetest.NewServer(t, "0.2.0", installerBody)
			srv.Sums = sums
			if sums == "" { // 空は updatetest が正しい値を補うので、空白だけにする
				srv.Sums = "\n"
			}
			dir := t.TempDir()
			_, err := srv.Stager(dir).Stage(t.Context(), latest(t, srv))
			require.Error(t, err)
			assert.NotErrorIs(t, err, update.ErrChecksumMismatch)
			assert.Empty(t, dirFiles(t, dir))
			assert.Zero(t, srv.Hits("/dl/"+name), "ハッシュが分からないうちはインストーラーを取らない")
		})
	}
}

func TestStageMissingAssets(t *testing.T) {
	for label, mod := range map[string]func(*updatetest.Server){
		"SHA256SUMS.txt が無い": func(s *updatetest.Server) { s.OmitSums = true },
		"インストーラーが無い":         func(s *updatetest.Server) { s.OmitInstaller = true },
	} {
		t.Run(label, func(t *testing.T) {
			srv := updatetest.NewServer(t, "0.2.0", installerBody)
			mod(srv)
			dir := t.TempDir()
			_, err := srv.Stager(dir).Stage(t.Context(), latest(t, srv))
			require.ErrorIs(t, err, update.ErrAssetMissing)
			assert.Empty(t, dirFiles(t, dir))
			assert.Zero(t, srv.TotalHits(), "何もダウンロードしない")
		})
	}
}

func TestStageRejectsBadVersion(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	rel := latest(t, srv)
	rel.Version = "../../evil"
	_, err := srv.Stager(t.TempDir()).Stage(t.Context(), rel)
	require.Error(t, err)
	assert.Zero(t, srv.TotalHits())
}

func TestStageSizeLimit(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	dir := t.TempDir()
	st := update.NewStager(update.StagerOptions{
		Dir: dir, HTTPClient: srv.Client(), AllowURL: func(*url.URL) bool { return true }, MaxInstallerBytes: int64(len(installerBody)) - 1,
	})
	_, err := st.Stage(t.Context(), latest(t, srv))
	require.Error(t, err)
	assert.Empty(t, dirFiles(t, dir))
	assert.Zero(t, srv.Hits("/dl/hitomado-v0.2.0-windows-amd64-setup.exe"), "添付の大きさが上限を超えるなら取りにいかない")
}

// 添付の宣言（size）より実際の本体が大きい・小さいときも使わない。
func TestStageRejectsSizeMismatch(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	rel := latest(t, srv)
	for i := range rel.Assets {
		if rel.Assets[i].Name == update.InstallerName("0.2.0") {
			rel.Assets[i].Size++
		}
	}
	dir := t.TempDir()
	_, err := srv.Stager(dir).Stage(t.Context(), rel)
	require.Error(t, err)
	assert.Empty(t, dirFiles(t, dir))
}

func TestStageBodyOverLimitWhenSizeUnknown(t *testing.T) {
	big := make([]byte, 4096)
	srv := updatetest.NewServer(t, "0.2.0", big)
	rel := latest(t, srv)
	for i := range rel.Assets {
		rel.Assets[i].Size = 0 // 宣言なし
	}
	dir := t.TempDir()
	st := update.NewStager(update.StagerOptions{
		Dir: dir, HTTPClient: srv.Client(), AllowURL: func(*url.URL) bool { return true }, MaxInstallerBytes: 1024,
	})
	_, err := st.Stage(t.Context(), rel)
	require.Error(t, err)
	assert.Empty(t, dirFiles(t, dir))
}

func TestStageRejectsPlainHTTP(t *testing.T) {
	var hits int
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; _, _ = w.Write(installerBody) }))
	t.Cleanup(plain.Close)
	rel := update.Release{Version: "0.2.0", Assets: []update.Asset{
		{Name: update.InstallerName("0.2.0"), URL: plain.URL + "/setup.exe"},
		{Name: update.SumsAsset, URL: plain.URL + "/SHA256SUMS.txt"},
	}}
	st := update.NewStager(update.StagerOptions{Dir: t.TempDir(), AllowURL: func(*url.URL) bool { return true }})
	_, err := st.Stage(t.Context(), rel)
	require.Error(t, err)
	assert.Zero(t, hits, "HTTPS 以外へは接続しない")
}

func TestStageRejectsRedirectToDisallowedHost(t *testing.T) {
	evil := updatetest.NewServer(t, "0.2.0", installerBody)
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	srv.RedirectTo = evil.URL
	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	dir := t.TempDir()
	// 許可は最初の接続先（srv）だけ。evil は同じ 127.0.0.1 でもポートが違う別の接続先として扱いたいので、ホスト:ポートで比べる。
	st := update.NewStager(update.StagerOptions{
		Dir: dir, HTTPClient: srv.Client(), AllowURL: func(u *url.URL) bool { return u.Host == srvURL.Host },
	})
	_, err = st.Stage(t.Context(), latest(t, srv))
	require.Error(t, err)
	assert.Zero(t, evil.TotalHits(), "許可していない接続先へのリダイレクトは追従しない")
	assert.Empty(t, dirFiles(t, dir))
}

func TestStageRejectsRedirectToPlainHTTP(t *testing.T) {
	var hits int
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; _, _ = w.Write(installerBody) }))
	t.Cleanup(plain.Close)
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	srv.RedirectTo = plain.URL
	_, err := srv.Stager(t.TempDir()).Stage(t.Context(), latest(t, srv))
	require.Error(t, err)
	assert.Zero(t, hits, "HTTP へのリダイレクトは追従しない")
}

func TestStageDefaultAllowsOnlyGitHubHosts(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody) // 127.0.0.1 は既定では許可されない
	st := update.NewStager(update.StagerOptions{Dir: t.TempDir(), HTTPClient: srv.Client()})
	_, err := st.Stage(t.Context(), latest(t, srv))
	require.Error(t, err)
	assert.Zero(t, srv.TotalHits())
}

func TestStageReusesVerifiedFileAndReplacesTamperedOne(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	dir := t.TempDir()
	st := srv.Stager(dir)
	rel := latest(t, srv)
	name := update.InstallerName("0.2.0")

	first, err := st.Stage(t.Context(), rel)
	require.NoError(t, err)
	_, err = st.Stage(t.Context(), rel)
	require.NoError(t, err)
	assert.Equal(t, 1, srv.Hits("/dl/"+name), "照合済みなら取り直さない")

	require.NoError(t, os.WriteFile(first.Path, []byte("tampered"), 0o600))
	again, err := st.Stage(t.Context(), rel)
	require.NoError(t, err)
	assert.Equal(t, 2, srv.Hits("/dl/"+name), "壊れた・差し替えられたファイルは使わず取り直す")
	body, err := os.ReadFile(again.Path)
	require.NoError(t, err)
	assert.Equal(t, installerBody, body)
}

func TestStageTimeout(t *testing.T) {
	slow := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(slow.Close)
	client := slow.Client()
	client.Timeout = 100 * time.Millisecond
	rel := update.Release{Version: "0.2.0", Assets: []update.Asset{
		{Name: update.InstallerName("0.2.0"), URL: slow.URL + "/a"}, {Name: update.SumsAsset, URL: slow.URL + "/b"},
	}}
	dir := t.TempDir()
	st := update.NewStager(update.StagerOptions{Dir: dir, HTTPClient: client, AllowURL: func(*url.URL) bool { return true }})
	_, err := st.Stage(t.Context(), rel)
	require.Error(t, err)
	assert.Empty(t, dirFiles(t, dir))
}

func TestStageContextCancelled(t *testing.T) {
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	rel := latest(t, srv)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := srv.Stager(t.TempDir()).Stage(ctx, rel)
	require.ErrorIs(t, err, context.Canceled)
}

// 失敗のエラー文面（ログ・画面）に、リダイレクト先の署名つきクエリを残さない。
func TestStageErrorDoesNotLeakQuery(t *testing.T) {
	closed := httptest.NewTLSServer(http.NotFoundHandler())
	base := closed.URL
	srv := updatetest.NewServer(t, "0.2.0", installerBody)
	srv.RedirectTo = base
	closed.Close()
	_, err := srv.Stager(t.TempDir()).Stage(t.Context(), latest(t, srv))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "dummy-token")
	assert.NotContains(t, err.Error(), "sig=")
}
