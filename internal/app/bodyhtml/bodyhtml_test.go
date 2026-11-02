package bodyhtml_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
)

func TestTextToHTML(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"空":            {"", ""},
		"空白のみ":         {" \n\t\n ", ""},
		"単一行":          {"こんにちは", "<p>こんにちは</p>"},
		"改行は br":       {"a\nb", "<p>a<br>b</p>"},
		"空行は段落":        {"a\n\nb", "<p>a</p><p>b</p>"},
		"空白付き空行も段落":    {"a\n \t\n\n\nb", "<p>a</p><p>b</p>"},
		"CRLF":         {"a\r\nb\r\n\r\nc", "<p>a<br>b</p><p>c</p>"},
		"CR のみ":        {"a\rb", "<p>a<br>b</p>"},
		"前後の空白は除く":     {"\n\n  a  \n\n", "<p>a</p>"},
		"タグはエスケープ":     {"<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>"},
		"& と引用符はエスケープ": {`a & "b" 'c'`, "<p>a &amp; &#34;b&#34; &#39;c&#39;</p>"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, bodyhtml.TextToHTML(c.in)) })
	}
}

func TestUserBodyHTMLRemovesSignatureAndQuote(t *testing.T) {
	in := `<p>本文</p><div data-hitomado-signature data-account-id="a1">-- <br>署名</div>` +
		`<blockquote data-hitomado-quote><p>引用</p></blockquote>`
	got := bodyhtml.UserBodyHTML(in)
	assert.Equal(t, "<p>本文</p>", got)

	nested := `<div><p>外</p><div data-hitomado-signature>-- <br>署名</div></div>`
	assert.Equal(t, "<div><p>外</p></div>", bodyhtml.UserBodyHTML(nested))

	assert.Equal(t, "", bodyhtml.UserBodyHTML(`<div data-hitomado-signature>sig</div>`))
	assert.Equal(t, "", bodyhtml.UserBodyHTML(""))
	assert.Equal(t, "素のテキスト", bodyhtml.UserBodyHTML("素のテキスト"))
}

func TestIsEmptyBoundaries(t *testing.T) {
	empty := []string{
		"",
		"   ",
		"<p></p>",
		"<p> </p><br>",
		"<p><br></p>",
		"<p>&nbsp;</p>",
		`<div data-hitomado-signature data-account-id="a1">-- <br>署名</div>`,
		`<p></p><div data-hitomado-signature>-- <br>署名</div><blockquote data-hitomado-quote><p>引用本文</p></blockquote>`,
		`<blockquote data-hitomado-quote><p>引用</p></blockquote>`,
	}
	for _, in := range empty {
		assert.True(t, bodyhtml.IsEmpty(in), "%q は実質空", in)
	}
	notEmpty := []string{
		"a",
		"<p>a</p>",
		"<p>こんにちは</p><div data-hitomado-signature>-- <br>署名</div>",
		`<div data-hitomado-signature>sig</div><p>署名の後に書いた</p>`,
		"<ul><li>x</li></ul>",
		`<a href="https://x.example">link</a>`,
	}
	for _, in := range notEmpty {
		assert.False(t, bodyhtml.IsEmpty(in), "%q は空でない", in)
	}
}

func TestStripMarkers(t *testing.T) {
	in := `<p class="x">本文</p><div data-hitomado-signature data-account-id="a1">-- <br>署名</div>` +
		`<blockquote data-hitomado-quote><p>引用 &amp; <a href="https://x.example/?a=1&amp;b=2">リンク</a></p></blockquote>`
	got := bodyhtml.StripMarkers(in)
	assert.NotContains(t, got, "data-hitomado")
	assert.NotContains(t, got, "data-account-id")
	assert.Contains(t, got, `<p class="x">本文</p>`)
	assert.Contains(t, got, "<div>-- <br>署名</div>")
	assert.Contains(t, got, "<blockquote><p>引用 &amp; ")
	assert.Contains(t, got, `<a href="https://x.example/?a=1&amp;b=2">リンク</a>`)
}

func TestStripMarkersLeavesPlainHTMLUntouched(t *testing.T) {
	in := `<p>a<br/>b</p><!-- c --><img src="x" alt="y"/>`
	assert.Equal(t, in, bodyhtml.StripMarkers(in))
	assert.Equal(t, "", bodyhtml.StripMarkers(""))
}

func TestStripMarkersCaseInsensitive(t *testing.T) {
	got := bodyhtml.StripMarkers(`<div DATA-HITOMADO-SIGNATURE DATA-ACCOUNT-ID="a">s</div>`)
	assert.NotContains(t, got, "DATA-")
	assert.Contains(t, got, ">s</div>")
}
