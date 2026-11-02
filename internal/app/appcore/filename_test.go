package appcore_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
)

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"normal.txt":       "normal.txt",
		`a<b>c:d"e|f.txt`:  "a_b_c_d_e_f.txt",
		`..\..\evil.exe`:   ".._.._evil.exe",
		"line\nbreak.txt":  "line_break.txt",
		"trailing. . ":     "trailing",
		"":                 "attachment",
		"   ":              "attachment",
		"...":              "attachment",
		"CON":              "_CON",
		"nul.txt":          "_nul.txt",
		"日本語の資料.pdf":       "日本語の資料.pdf",
		"bad\xffbytes.txt": "bad_bytes.txt",
	}
	for in, want := range cases {
		assert.Equal(t, want, appcore.SanitizeFilename(in), "入力 %q", in)
	}
}

func TestSanitizeFilenameLimitsBytesAndKeepsExtension(t *testing.T) {
	long := strings.Repeat("あ", 200) + ".pdf" // 600+ バイト
	got := appcore.SanitizeFilename(long)
	assert.LessOrEqual(t, len(got), 255)
	assert.True(t, strings.HasSuffix(got, ".pdf"), "拡張子を保持する: %q", got)
	assert.True(t, utf8.ValidString(got), "ルーン境界で切る")
	assert.True(t, strings.HasPrefix(got, "あああ"))

	ascii := strings.Repeat("a", 400) + ".tar.gz"
	got = appcore.SanitizeFilename(ascii)
	assert.LessOrEqual(t, len(got), 255)
	assert.True(t, strings.HasSuffix(got, ".gz"))

	// 長すぎる「拡張子」は拡張子として扱わず、全体を切る。
	got = appcore.SanitizeFilename("x." + strings.Repeat("e", 400))
	assert.LessOrEqual(t, len(got), 255)

	// 予約名の接頭辞を足しても上限を超えない。
	got = appcore.SanitizeFilename("con." + strings.Repeat("e", 400))
	assert.LessOrEqual(t, len(got), 255)
	assert.True(t, strings.HasPrefix(got, "_con"))

	exact := strings.Repeat("b", 251) + ".txt" // 255 バイト丁度
	assert.LessOrEqual(t, len(appcore.SanitizeFilename(exact)), 255)
}
