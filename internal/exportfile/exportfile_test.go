package exportfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathTarget(t *testing.T) {
	tg := NewPathTarget("/x/y.pdf")
	assert.Equal(t, "/x/y.pdf", tg.Path())
	loc, err := tg.Commit(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/x/y.pdf", loc)
	tg.Close()
}

func TestMIMEForName(t *testing.T) {
	cases := map[string]string{
		"a.pdf": "application/pdf", "A.PDF": "application/pdf", "hitomado-logs-1.zip": "application/zip",
		"x.log": "text/plain", "p.jpeg": "image/jpeg", "s.xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"noext": "application/octet-stream", "weird.zzzunknown": "application/octet-stream", "": "application/octet-stream",
		"dir.pdf/": "application/octet-stream",
	}
	for name, want := range cases {
		assert.Equal(t, want, MIMEForName(name), "name=%q", name)
	}
}

func TestDisplayNameIsSafe(t *testing.T) {
	cases := map[string]string{
		"report.pdf":                      "report.pdf",
		"../../etc/passwd":                "passwd",
		`..\..\Windows\win.ini`:           "win.ini",
		"/abs/path/file.txt":              "file.txt",
		"a/b/..":                          "attachment", // 末尾がドットだけ
		"..":                              "attachment",
		"":                                "attachment",
		"con.txt":                         "_con.txt",
		"a\x00b\nc.pdf":                   "a_b_c.pdf",
		"re:port*?.pdf":                   "re_port__.pdf",
		"日本語の請求書.pdf":                     "日本語の請求書.pdf",
		strings.Repeat("あ", 200) + ".pdf": "", // 長さの検査は下
	}
	for in, want := range cases {
		got := DisplayName(in)
		if want != "" {
			assert.Equal(t, want, got, "in=%q", in)
		}
		assert.LessOrEqual(t, len(got), 255, "in=%q", in)
		assert.True(t, utf8.ValidString(got))
		assert.NotContains(t, got, "/")
		assert.NotContains(t, got, `\`)
		assert.NotEqual(t, "..", got)
		assert.True(t, strings.HasSuffix(got, ".pdf") || want != "" || in == "", "拡張子を保つ: %q", got)
	}
}

func TestOpenRequest(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.log")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))

	m, err := OpenRequest(f)
	require.NoError(t, err)
	assert.Equal(t, "text/plain", m)

	_, err = OpenRequest(dir)
	assert.ErrorIs(t, err, ErrFolderNotSupported)

	_, err = OpenRequest(filepath.Join(dir, "missing.log"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestParseResult(t *testing.T) {
	r, ok := ParseResult(map[string]any{"id": float64(3), "status": "saved", "name": "x.pdf"})
	require.True(t, ok)
	assert.Equal(t, Result{ID: 3, Status: StatusSaved, Name: "x.pdf"}, r)

	r, ok = ParseResult(map[string]any{"id": float64(4), "status": "failed", "error": "write_failed"})
	require.True(t, ok)
	assert.Equal(t, Result{ID: 4, Status: StatusFailed, Error: "write_failed"}, r)

	for _, bad := range []any{
		nil, "x", map[string]any{}, map[string]any{"id": "1", "status": "saved"},
		map[string]any{"id": 1.5, "status": "saved"}, map[string]any{"id": float64(1), "status": "weird"},
		map[string]any{"id": float64(1)},
	} {
		_, ok := ParseResult(bad)
		assert.False(t, ok, "bad=%v", bad)
	}
}

func TestParseEnvelope(t *testing.T) {
	v, err := ParseEnvelope(`{"ok":true,"value":"/cache/exports"}`)
	require.NoError(t, err)
	assert.Equal(t, "/cache/exports", v)

	_, err = ParseEnvelope(`{"ok":false,"error":"busy"}`)
	var be *BridgeError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "busy", be.Code)

	_, err = ParseEnvelope(`not json`)
	assert.Error(t, err)
	var be2 *BridgeError
	assert.NotErrorAs(t, err, &be2)
}

func TestASCIIJSON(t *testing.T) {
	in := map[string]any{"name": "請求書😀<&>.pdf", "id": 1}
	s, err := ASCIIJSON(in)
	require.NoError(t, err)
	for i := 0; i < len(s); i++ {
		require.Less(t, s[i], byte(0x80), "ASCII だけ: %q", s)
	}
	assert.Contains(t, s, `\ud83d\ude00`, "BMP 外はサロゲートペア")
	var back map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &back))
	assert.Equal(t, "請求書😀<&>.pdf", back["name"], "Java が戻せる（標準の JSON としてそのまま読める）")
}

func TestBridgeErrorMapsJavaCodes(t *testing.T) {
	assert.ErrorIs(t, &BridgeError{Code: "busy"}, ErrBusy)
	assert.ErrorIs(t, &BridgeError{Code: "no_app"}, ErrNoApp)
	assert.ErrorIs(t, &BridgeError{Code: "is_directory"}, ErrFolderNotSupported)
	assert.NotErrorIs(t, &BridgeError{Code: "bad_path"}, ErrNoApp)
	assert.NotErrorIs(t, &BridgeError{Code: "no_app"}, ErrBusy)
	assert.NotContains(t, (&BridgeError{Code: "bad_path"}).Error(), "/", "コードだけでパスを含まない")
}
