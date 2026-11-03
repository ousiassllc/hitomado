package logging

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingReaderAt struct {
	r     *bytes.Reader
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err //nolint:wrapcheck // テスト用の素通し
}

func collectReverse(t *testing.T, data string, stopAfter int) []string {
	t.Helper()
	var got []string
	err := eachLineReverse(bytes.NewReader([]byte(data)), int64(len(data)), func(l []byte) bool {
		got = append(got, string(l))
		return stopAfter <= 0 || len(got) < stopAfter
	})
	require.NoError(t, err)
	return got
}

func TestEachLineReverseHandlesEdgeCases(t *testing.T) {
	assert.Equal(t, []string{"c", "b", "a"}, collectReverse(t, "a\nb\nc\n", 0))
	assert.Equal(t, []string{"c", "b", "a"}, collectReverse(t, "a\nb\nc", 0), "末尾に改行が無い最後の行も読む")
	assert.Equal(t, []string{"c", "b", "a"}, collectReverse(t, "a\r\nb\r\n\r\nc\r\n", 0), "CRLF と空行")
	assert.Equal(t, []string{"c", "b"}, collectReverse(t, "a\nb\nc\n", 2), "false で止まる")
	assert.Empty(t, collectReverse(t, "", 0))

	long := strings.Repeat("x", maxLineBytes+1)
	assert.Equal(t, []string{"z", "a"}, collectReverse(t, "a\n"+long+"\nz\n", 0), "長すぎる行は飛ばして前後を読む")
	assert.Equal(t, []string{"z", "a"}, collectReverse(t, "a\n"+long+"\nz", 0), "長すぎる行（チャンクをまたぐ）")
	ok := strings.Repeat("y", maxLineBytes)
	assert.Equal(t, []string{ok, "a"}, collectReverse(t, "a\n"+ok+"\n", 0), "上限ちょうどの行は読む（チャンクをまたぐ）")
}

func TestEachLineReverseSpansChunks(t *testing.T) {
	var lines []string
	var b strings.Builder
	for i := range 5000 { // チャンク（64KiB）を何度もまたぐ量
		l := fmt.Sprintf("line-%05d-%s", i, strings.Repeat("p", i%97))
		lines = append(lines, l)
		b.WriteString(l + "\n")
	}
	got := collectReverse(t, b.String(), 0)
	slices.Reverse(lines)
	assert.Equal(t, lines, got)
}

func TestEachLineReverseStopsWithoutReadingWholeFile(t *testing.T) {
	var b strings.Builder
	for i := range 100000 {
		fmt.Fprintf(&b, "line-%06d %s\n", i, strings.Repeat("p", 90))
	}
	data := []byte(b.String())
	cr := &countingReaderAt{r: bytes.NewReader(data)}
	n := 0
	require.NoError(t, eachLineReverse(cr, int64(len(data)), func([]byte) bool { n++; return n < 10 }))
	assert.Equal(t, 10, n)
	assert.LessOrEqual(t, cr.bytes, int64(reverseChunkBytes), "10 行ぶんなら末尾の 1 チャンクだけ読む（全 %d バイトは読まない）", len(data))
}
