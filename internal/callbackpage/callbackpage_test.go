package callbackpage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// フォントは Sora + Noto Sans JP、サイズ・行間はガイドのスケール、見出しは SemiBold 600（デザインガイド T1・T2・T3・T4）。
func TestPageTypography(t *testing.T) {
	_, body := render(t, "x")
	for _, want := range []string{
		`font-family:Sora,"Noto Sans JP",sans-serif`,
		"h1{font-size:18px;line-height:1.7;font-weight:600;",
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

// tokensCSS は frontend/src/styles/tokens.css を読み、ライト（:root）とダーク（prefers-color-scheme: dark の :root）の
// トークン（--name: value）を返す。完了ページの値の複製がトークンとずれていないかを検証するための読み取り。
func tokensCSS(t *testing.T) (light, dark map[string]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	const darkStart, darkEnd = "@media (prefers-color-scheme: dark)", "@media (prefers-reduced-motion"
	i, j := strings.Index(css, darkStart), strings.Index(css, darkEnd)
	if i < 0 || j < i {
		t.Fatal("dark block is not found in tokens.css")
	}
	decl := regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+):\s*([^;]+);`)
	parse := func(s string) map[string]string {
		m := map[string]string{}
		for _, d := range decl.FindAllStringSubmatch(s, -1) {
			m[d[1]] = strings.TrimSpace(d[2])
		}
		return m
	}
	return parse(css[:i]), parse(css[i:j])
}

// 完了ページが複製している値（色・フォント・サイズ・行間・角丸・余白）は tokens.css と一致する。
// トークンを変えたらこのテストが落ちるので、callbackpage.go の page も合わせて直す（ビルド時の共有はしない。docs/components/overview.md §5.3）。
func TestPageValuesMatchTokens(t *testing.T) {
	_, body := render(t, "x")
	light, dark := tokensCSS(t)

	root := regexp.MustCompile(`:root\{([^}]*)\}`).FindAllStringSubmatch(body, -1)
	if len(root) != 2 {
		t.Fatalf(":root rules = %d, want 2 (light, dark)", len(root))
	}
	for i, want := range []map[string]string{light, dark} {
		got := regexp.MustCompile(`(--[a-z]+):([^;]+)`).FindAllStringSubmatch(root[i][1], -1)
		if len(got) != 4 {
			t.Fatalf("scheme %d: %d tokens, want 4 (ink, bone, surface, border)", i, len(got))
		}
		for _, g := range got {
			if g[2] != want[g[1]] {
				t.Errorf("scheme %d: %s = %s, tokens.css has %s", i, g[1], g[2], want[g[1]])
			}
		}
	}

	// トークンが改名・削除されると下の期待値が空文字を含んで常に一致してしまうので、先に存在を確かめる。
	for _, k := range []string{"--font-sans", "--font-size-lg", "--line-height-lg", "--font-size-base", "--line-height-base", "--s-6", "--r-lg", "--s-3"} {
		if light[k] == "" {
			t.Fatalf("tokens.css has no %s", k)
		}
	}

	// フォントはトークンの family（"Sora Variable" 等は同梱フォントの @font-face 名）から " Variable" を外した、端末のフォント名。
	fam := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(light["--font-sans"], " Variable", ""), `"Sora"`, "Sora"), ", ", ",")
	if want := "font-family:" + fam; !strings.Contains(body, want) {
		t.Errorf("font-family does not match --font-sans: want %q", want)
	}
	for _, want := range []string{
		"font-size:" + light["--font-size-lg"] + ";line-height:" + light["--line-height-lg"],
		"font-size:" + light["--font-size-base"] + ";line-height:" + light["--line-height-base"],
		"padding:" + light["--s-6"],
		"border-radius:" + light["--r-lg"],
		"margin:0 0 " + light["--s-3"],
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q (tokens.css と不一致)", want)
		}
	}
}
