package callbackpage

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func render(t *testing.T, msg string) (http.Header, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	Write(rec, msg)
	return rec.Header(), rec.Body.String()
}

func TestWriteHeadersAndEscape(t *testing.T) {
	h, body := render(t, `<script>alert(1)</script>`)
	if got := h.Get("Content-Security-Policy"); got != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Errorf("CSP = %q", got)
	}
	if h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", h)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("msg is not escaped: %s", body)
	}
}

// 色はトークン（:root の変数）経由だけにし、ライト・ダークの両方を持つ（デザインガイド C2・C5）。
func TestPageColorsComeFromTokens(t *testing.T) {
	_, body := render(t, "x")
	if !strings.Contains(body, "@media (prefers-color-scheme:dark)") {
		t.Error("dark scheme branch is missing")
	}
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,6}\b`)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, ":root{") || strings.HasPrefix(line, "@media") {
			continue // トークンの定義行
		}
		if hex.MatchString(line) || strings.Contains(line, "rgb") {
			t.Errorf("hard-coded color outside token definitions: %s", line)
		}
	}
	for _, want := range []string{"background:var(--bone)", "color:var(--ink)", "background:var(--surface)", "1px solid var(--border)"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// フォントは Sora + Noto Sans JP、サイズ・行間はガイドのスケール（デザインガイド T1・T3・T4）。
func TestPageTypography(t *testing.T) {
	_, body := render(t, "x")
	for _, want := range []string{
		`font-family:Sora,"Noto Sans JP",sans-serif`,
		"h1{font-size:18px;line-height:1.7;",
		"p{margin:0;font-size:16px;line-height:1.8}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"font-family:sans-serif", "1.2rem", "line-height:1.6"} {
		if strings.Contains(body, bad) {
			t.Errorf("unexpected %q", bad)
		}
	}
}

// カードは影なしの 1px solid var(--border)・--r-lg（12px）・--pad-card（24px）（デザインガイド S1・S3）。
func TestPageCardShape(t *testing.T) {
	_, body := render(t, "x")
	m := regexp.MustCompile(`main\{([^}]*)\}`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("main rule is missing")
	}
	for _, want := range []string{"padding:24px", "border-radius:12px", "border:1px solid var(--border)"} {
		if !strings.Contains(m[1], want) {
			t.Errorf("main rule lacks %q: %s", want, m[1])
		}
	}
	if strings.Contains(body, "shadow") {
		t.Error("card must not have a shadow")
	}
}
