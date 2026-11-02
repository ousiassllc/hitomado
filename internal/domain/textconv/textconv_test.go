package textconv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTMLToText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"段落と改行", "<p>こんにちは</p><p>世界<br>です</p>", "こんにちは\n\n世界\nです"},
		{"リンクはURL付き", `<a href="https://example.com/x">資料</a>`, "資料 (https://example.com/x)"},
		{"表示文字とURLが同じなら重複しない", `<a href="https://e.com">https://e.com</a>`, "https://e.com"},
		{"箇条書き", "<ul><li>A</li><li>B</li></ul>", "- A\n- B"},
		{"script と style は除去", "<style>p{}</style><script>alert(1)</script><p>本文</p>", "本文"},
		{"エンティティ解除", "<p>a &amp; b &lt;c&gt;</p>", "a & b <c>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, HTMLToText(c.in))
		})
	}
}

func TestPreview(t *testing.T) {
	assert.Equal(t, "あいう…", Preview("<p>あいうえお</p>", 3))
	assert.Equal(t, "あ い", Preview("<p>あ</p>\n<p>い</p>", 10))
}
