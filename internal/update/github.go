package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	defaultRepo    = "ousiassllc/hitomado"
	defaultUA      = "Hitomado"
	httpTimeout    = 30 * time.Second
	maxBodyBytes   = 1 << 20
)

// Options は Checker の構成。ゼロ値のフィールドは既定値になる。
type Options struct {
	HTTPClient *http.Client
	BaseURL    string // 既定 https://api.github.com
	Repo       string // 既定 ousiassllc/hitomado
	UserAgent  string // 例: Hitomado/0.1.0。既定 Hitomado
}

type github struct {
	hc      *http.Client
	baseURL string
	repo    string
	ua      string
}

// NewGitHub は GitHub Releases の最新リリースを未認証で取得する Checker を返す。
// 送るのは IP・User-Agent・リクエスト行のみ（メール・アカウント情報は含めない）。
func NewGitHub(opts Options) Checker {
	g := &github{hc: opts.HTTPClient, baseURL: opts.BaseURL, repo: opts.Repo, ua: opts.UserAgent}
	if g.hc == nil {
		g.hc = &http.Client{Timeout: httpTimeout}
	}
	if g.baseURL == "" {
		g.baseURL = defaultBaseURL
	}
	g.baseURL = strings.TrimRight(g.baseURL, "/")
	if g.repo == "" {
		g.repo = defaultRepo
	}
	if g.ua == "" {
		g.ua = defaultUA
	}
	return g
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// Latest は最新リリースを返す。Version は tag_name の先頭 v を除いたもの。
func (g *github) Latest(ctx context.Context) (Release, error) {
	endpoint := g.baseURL + "/repos/" + g.repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, fmt.Errorf("update: 要求を作れません: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", g.ua)

	resp, err := g.hc.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("update: 最新版の確認に失敗しました: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		return Release{}, fmt.Errorf("update: GitHub が HTTP %d を返しました", resp.StatusCode)
	}
	var body releaseResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("update: 応答を解釈できません: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if version == "" {
		return Release{}, errors.New("update: 応答に tag_name がありません")
	}
	rel := Release{Version: version, URL: body.HTMLURL}
	for _, a := range body.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	return rel, nil
}
