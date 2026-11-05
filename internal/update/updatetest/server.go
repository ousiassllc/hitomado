// Package updatetest は自動アップデート（internal/update）のテスト用の、偽の GitHub Release。
package updatetest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/ousiassllc/hitomado/internal/update"
)

// Server は Releases API とファイルの配信を 1 つの TLS サーバーで行う偽の Release。
// 最新の版 Version のインストーラー（Installer の内容）と SHA256SUMS.txt を添付として持つ。
type Server struct {
	*httptest.Server

	mu sync.Mutex
	// Version は latest の版（先頭の v なし）。
	Version string
	// Installer はインストーラーとして配信する内容。
	Installer []byte
	// Sums は SHA256SUMS.txt の内容。空なら Installer の正しいハッシュの行を返す。
	Sums string
	// OmitSums・OmitInstaller は添付一覧からそれぞれを外す。
	OmitSums, OmitInstaller bool
	// RedirectTo は空でなければ、ダウンロードを 302 でそのベース URL へ飛ばす（GitHub が配信元へ飛ばす動きの再現）。
	RedirectTo string

	hits map[string]int
}

// NewServer は TLS の偽 Release を起動する（t.Cleanup で閉じる）。
func NewServer(t testing.TB, version string, installer []byte) *Server {
	t.Helper()
	s := &Server{Version: version, Installer: installer, hits: map[string]int{}}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Checker は偽の Release を見る update.Checker。
func (s *Server) Checker() update.Checker {
	return update.NewGitHub(update.Options{HTTPClient: s.Client(), BaseURL: s.URL, UserAgent: "Hitomado/test"})
}

// Stager は偽の Release の配信を許可した update.Stager（保存先は dir）。
func (s *Server) Stager(dir string) *update.Stager {
	return update.NewStager(update.StagerOptions{
		Dir: dir, HTTPClient: s.Client(), AllowURL: func(*url.URL) bool { return true },
	})
}

// Hits は path（例: "/dl/SHA256SUMS.txt"）への要求の回数。
func (s *Server) Hits(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// TotalHits は API 以外（添付ファイルの配信）への要求の合計。
func (s *Server) TotalHits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for p, c := range s.hits {
		if p != "/repos/ousiassllc/hitomado/releases/latest" {
			n += c
		}
	}
	return n
}

// Sum は data の SHA-256（小文字の 16 進）。
func Sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hits[r.URL.Path]++
	version, installer, sums, redirect := s.Version, s.Installer, s.Sums, s.RedirectTo
	omitSums, omitInstaller := s.OmitSums, s.OmitInstaller
	s.mu.Unlock()

	name := update.InstallerName(version)
	switch r.URL.Path {
	case "/repos/ousiassllc/hitomado/releases/latest":
		assets := ""
		if !omitInstaller {
			assets += fmt.Sprintf(`{"name":%q,"browser_download_url":"%s/dl/%s","size":%d}`, name, s.URL, name, len(installer))
		}
		if !omitSums {
			if assets != "" {
				assets += ","
			}
			assets += fmt.Sprintf(`{"name":%q,"browser_download_url":"%s/dl/%s","size":0}`, update.SumsAsset, s.URL, update.SumsAsset)
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"%s/releases/tag/v%s","assets":[%s]}`, version, s.URL, version, assets)
	case "/dl/" + name, "/dl/" + update.SumsAsset:
		if redirect != "" {
			http.Redirect(w, r, redirect+r.URL.Path+"?sig=dummy-token", http.StatusFound) //nolint:gosec // テスト用の偽サーバー（リダイレクト先は呼び出し側が決める）
			return
		}
		if r.URL.Path == "/dl/"+update.SumsAsset {
			if sums == "" {
				sums = Sum(installer) + "  " + name + "\n"
			}
			_, _ = w.Write([]byte(sums))
			return
		}
		_, _ = w.Write(installer)
	default:
		http.NotFound(w, r)
	}
}
