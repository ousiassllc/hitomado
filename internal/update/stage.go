package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// SumsAsset は Release に添付される、インストーラーの SHA-256 の一覧（release.yml が作る）。
	SumsAsset = "SHA256SUMS.txt"

	// DefaultMaxInstallerBytes はインストーラーのダウンロードサイズの上限（実物は約 14 MB）。
	DefaultMaxInstallerBytes int64 = 200 << 20
	// maxSumsBytes は SHA256SUMS.txt のサイズの上限。
	maxSumsBytes = 64 << 10
	// downloadTimeout はファイル 1 つのダウンロード全体の上限（独自のクライアントを渡されないときの既定）。
	downloadTimeout = 10 * time.Minute
	// maxRedirects はリダイレクトの追従回数の上限。
	maxRedirects = 5

	downloadPrefix = "download-"
	downloadSuffix = ".tmp"
)

var (
	// ErrChecksumMismatch はダウンロードしたファイルが SHA256SUMS.txt の値と一致しなかったことを表す。
	ErrChecksumMismatch = errors.New("update: ダウンロードしたファイルのハッシュが SHA256SUMS.txt と一致しません")
	// ErrAssetMissing は Release に必要な添付ファイルが無いことを表す。
	ErrAssetMissing = errors.New("update: Release に必要な添付ファイルがありません")

	versionRe   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	installerRe = regexp.MustCompile(`^hitomado-v([0-9]+\.[0-9]+\.[0-9]+)-windows-amd64-setup\.exe$`)
	sha256Re    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

// InstallerName は版 X.Y.Z の Windows インストーラーの添付ファイル名。
func InstallerName(version string) string {
	return "hitomado-v" + version + "-windows-amd64-setup.exe"
}

// Staged はダウンロードと SHA-256 の照合が済み、入れ替えを待つインストーラー。
type Staged struct {
	Version string
	Path    string
	SHA256  string // 小文字の 16 進
}

// StagerOptions は Stager の構成。ゼロ値のフィールドは既定値になる。
type StagerOptions struct {
	// Dir はダウンロードの保存先（作成する。利用者のデータ配下の専用フォルダ）。
	Dir string
	// HTTPClient は通信に使うクライアント。リダイレクトの制限は渡された値に関係なく掛ける（コピーして CheckRedirect を差し替える）。
	HTTPClient *http.Client
	// AllowURL は通信してよい URL（最初の要求とリダイレクト先）。https 以外は常に拒否する。既定は github.com と *.githubusercontent.com。
	AllowURL func(*url.URL) bool
	// MaxInstallerBytes はインストーラーのサイズの上限。既定は DefaultMaxInstallerBytes。
	MaxInstallerBytes int64
	UserAgent         string
}

// Stager は Release の Windows インストーラーを保存先へダウンロードし、SHA256SUMS.txt と照合する。
type Stager struct {
	dir      string
	hc       *http.Client
	allow    func(*url.URL) bool
	maxBytes int64
	ua       string
}

// NewStager は Stager を作る。
func NewStager(opts StagerOptions) *Stager {
	s := &Stager{dir: opts.Dir, allow: opts.AllowURL, maxBytes: opts.MaxInstallerBytes, ua: opts.UserAgent}
	if s.allow == nil {
		s.allow = defaultAllowURL
	}
	if s.maxBytes <= 0 {
		s.maxBytes = DefaultMaxInstallerBytes
	}
	if s.ua == "" {
		s.ua = defaultUA
	}
	client := http.Client{Timeout: downloadTimeout}
	if opts.HTTPClient != nil {
		client = *opts.HTTPClient
		if client.Timeout == 0 {
			client.Timeout = downloadTimeout
		}
	}
	client.CheckRedirect = s.checkRedirect
	s.hc = &client
	return s
}

// defaultAllowURL は Release の添付ファイルの配信元。github.com から release-assets.githubusercontent.com
// （以前は objects.githubusercontent.com）へリダイレクトされる。
func defaultAllowURL(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

func (s *Stager) checkURL(u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("update: HTTPS 以外の URL へは接続しません（%s）", u.Hostname())
	}
	if u.User != nil || !s.allow(u) {
		return fmt.Errorf("update: 許可していない接続先です（%s）", u.Hostname())
	}
	return nil
}

func (s *Stager) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return errors.New("update: リダイレクトが多すぎます")
	}
	return s.checkURL(req.URL)
}

// Stage は rel のインストーラーを Dir へダウンロードして照合する。照合が済んでから確定名へ移すので、
// 途中のファイル・照合に失敗したファイルが Staged として残ることはない（失敗時は破棄する）。
// 同じ版が照合済みで保存先にあれば、再ダウンロードしない。
func (s *Stager) Stage(ctx context.Context, rel Release) (Staged, error) {
	if !versionRe.MatchString(rel.Version) {
		return Staged{}, fmt.Errorf("update: 自動で入れ替えられない版です（%q）", rel.Version)
	}
	name := InstallerName(rel.Version)
	installer, ok := findAsset(rel.Assets, name)
	if !ok {
		return Staged{}, fmt.Errorf("%w（%s）", ErrAssetMissing, name)
	}
	sums, ok := findAsset(rel.Assets, SumsAsset)
	if !ok {
		return Staged{}, fmt.Errorf("%w（%s）", ErrAssetMissing, SumsAsset)
	}
	want, err := s.fetchChecksum(ctx, sums.URL, name)
	if err != nil {
		return Staged{}, err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Staged{}, fmt.Errorf("update: 保存先を作れません: %w", err)
	}
	final := filepath.Join(s.dir, name)
	if got, err := hashFile(final); err == nil {
		if got == want {
			return Staged{Version: rel.Version, Path: final, SHA256: want}, nil
		}
		_ = os.Remove(final) // 以前の破損・差し替えられたファイルは使わない
	}
	if err := s.download(ctx, installer, want, final); err != nil {
		return Staged{}, err
	}
	return Staged{Version: rel.Version, Path: final, SHA256: want}, nil
}

func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

func (s *Stager) get(ctx context.Context, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("update: URL を解釈できません: %w", err)
	}
	if err := s.checkURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("update: 要求を作れません: %w", err)
	}
	req.Header.Set("User-Agent", s.ua)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: ダウンロードに失敗しました: %w", scrubURL(err))
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("update: ダウンロード先が HTTP %d を返しました", resp.StatusCode)
	}
	return resp, nil
}

// scrubURL は *url.Error の URL からクエリ（リダイレクト先の一時的な署名つきトークン）を除く。ログ・エラー文面に残さないため。
func scrubURL(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	if u, perr := url.Parse(ue.URL); perr == nil {
		u.RawQuery, u.Fragment = "", ""
		return &url.Error{Op: ue.Op, URL: u.String(), Err: ue.Err}
	}
	return &url.Error{Op: ue.Op, URL: "(invalid)", Err: ue.Err}
}

// fetchChecksum は SHA256SUMS.txt から name の SHA-256（小文字の 16 進）を取り出す。
func (s *Stager) fetchChecksum(ctx context.Context, sumsURL, name string) (string, error) {
	resp, err := s.get(ctx, sumsURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSumsBytes+1))
	if err != nil {
		return "", fmt.Errorf("update: %s を読めません: %w", SumsAsset, err)
	}
	if len(body) > maxSumsBytes {
		return "", fmt.Errorf("update: %s が大きすぎます", SumsAsset)
	}
	return parseChecksum(string(body), name)
}

// parseChecksum は sha256sum 形式（`<64 桁の 16 進>  <ファイル名>`。バイナリ指定の `*` は許容）から name の行を探す。
func parseChecksum(sums, name string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if !sha256Re.MatchString(fields[0]) {
			return "", fmt.Errorf("update: %s の %s の行が正しい形式ではありません", SumsAsset, name)
		}
		return strings.ToLower(fields[0]), nil
	}
	return "", fmt.Errorf("%w（%s に %s の行がありません）", ErrAssetMissing, SumsAsset, name)
}

// download は a を一時ファイルへ保存しながら SHA-256 を計算し、want と一致したときだけ final へ移す。
func (s *Stager) download(ctx context.Context, a Asset, want, final string) (err error) {
	if a.Size > s.maxBytes {
		return fmt.Errorf("update: %s が大きすぎます（%d バイト）", a.Name, a.Size)
	}
	resp, err := s.get(ctx, a.URL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.ContentLength > s.maxBytes {
		return fmt.Errorf("update: %s が大きすぎます（%d バイト）", a.Name, resp.ContentLength)
	}

	tmp, err := os.CreateTemp(s.dir, downloadPrefix+"*"+downloadSuffix)
	if err != nil {
		return fmt.Errorf("update: 一時ファイルを作れません: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, s.maxBytes+1))
	if err != nil {
		return fmt.Errorf("update: ダウンロードに失敗しました: %w", err)
	}
	if n > s.maxBytes {
		return fmt.Errorf("update: %s が上限（%d バイト）を超えました", a.Name, s.maxBytes)
	}
	if a.Size > 0 && n != a.Size {
		return fmt.Errorf("update: %s のサイズが Release の値（%d バイト）と違います（%d バイト）", a.Name, a.Size, n)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return ErrChecksumMismatch
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("update: 保存に失敗しました: %w", err)
	}
	if err = os.Rename(tmp.Name(), final); err != nil {
		return fmt.Errorf("update: 保存に失敗しました: %w", err)
	}
	return nil
}

// hashFile は path の SHA-256（小文字の 16 進）を返す。
func hashFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // 保存先フォルダ内の固定名
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// cleanStaging は dir から、途中のダウンロードと、current 以下の版（入れ替え済み・不要）のインストーラーを消す。
func cleanStaging(dir, current string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		stale := strings.HasPrefix(name, downloadPrefix) && strings.HasSuffix(name, downloadSuffix)
		if m := installerRe.FindStringSubmatch(name); m != nil {
			stale = !IsNewer(current, m[1])
		}
		if stale {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
