package quote

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

var src = model.SourceMessage{
	From:     model.Address{Name: "Alice", Email: "alice@x.com"},
	To:       []model.Address{{Name: "Me", Email: "me@x.com"}, {Email: "bob@x.com"}},
	Cc:       []model.Address{{Email: "carol@x.com"}},
	Subject:  "打ち合わせ",
	Date:     time.Date(2026, 1, 2, 15, 4, 59, 0, time.FixedZone("JST", 9*3600)),
	BodyHTML: "<p>こんにちは</p><p>よろしく</p>",
	BodyText: "こんにちは\n\nよろしく",
}

func TestBuildReplyHTML(t *testing.T) {
	html, text := Build(model.ModeReply, src)
	assert.Equal(t,
		`<blockquote data-hitomado-quote><p>2026-01-02 15:04 Alice &lt;alice@x.com&gt; が書きました:</p><p>こんにちは</p><p>よろしく</p></blockquote>`,
		html)
	assert.Equal(t, "2026-01-02 15:04 Alice <alice@x.com> が書きました:\n> こんにちは\n>\n> よろしく", text)
	again, _ := Build(model.ModeReplyAll, src)
	assert.Equal(t, html, again, "reply_all も同じ")
}

func TestBuildReplyPlainTextOnly(t *testing.T) {
	s := src
	s.BodyHTML = ""
	s.BodyText = "a <b> & c\r\nline2\r\n"
	html, text := Build(model.ModeReply, s)
	assert.Contains(t, html, "<p>2026-01-02 15:04 Alice &lt;alice@x.com&gt; が書きました:</p>a &lt;b&gt; &amp; c<br>line2</blockquote>")
	assert.Equal(t, "2026-01-02 15:04 Alice <alice@x.com> が書きました:\n> a <b> & c\n> line2", text)
}

func TestBuildHTMLOnlyTextFromHTML(t *testing.T) {
	s := src
	s.BodyText = ""
	s.BodyHTML = `<p>a</p><p>see <a href="https://e.com">here</a></p>`
	_, text := Build(model.ModeReply, s)
	assert.Contains(t, text, "> a")
	assert.Contains(t, text, "> see here (https://e.com)")
}

func TestBuildForward(t *testing.T) {
	html, text := Build(model.ModeForward, src)
	assert.Equal(t,
		`<blockquote data-hitomado-quote><p>---------- 転送されたメッセージ ----------<br>差出人: Alice &lt;alice@x.com&gt;<br>日時: 2026-01-02 15:04<br>件名: 打ち合わせ<br>宛先: Me &lt;me@x.com&gt;, bob@x.com<br>Cc: carol@x.com</p><p>こんにちは</p><p>よろしく</p></blockquote>`,
		html)
	assert.Equal(t, "---------- 転送されたメッセージ ----------\n差出人: Alice <alice@x.com>\n日時: 2026-01-02 15:04\n件名: 打ち合わせ\n宛先: Me <me@x.com>, bob@x.com\nCc: carol@x.com\n\nこんにちは\n\nよろしく", text)
}

func TestBuildForwardOmitsEmptyHeaders(t *testing.T) {
	s := model.SourceMessage{From: model.Address{Email: "a@x.com"}, Subject: "s", BodyText: "t"}
	html, text := Build(model.ModeForward, s)
	assert.NotContains(t, html, "宛先")
	assert.NotContains(t, html, "Cc:")
	assert.NotContains(t, html, "日時")
	assert.Equal(t, "---------- 転送されたメッセージ ----------\n差出人: a@x.com\n件名: s\n\nt", text)
}

func TestBuildDateUsesSourceZone(t *testing.T) {
	s := src
	s.Date = time.Date(2026, 1, 2, 23, 30, 0, 0, time.UTC)
	html, _ := Build(model.ModeReply, s)
	assert.Contains(t, html, "2026-01-02 23:30")
}

func TestBuildNew(t *testing.T) {
	html, text := Build(model.ModeNew, src)
	assert.Empty(t, html)
	assert.Empty(t, text)
}

func TestBuildSanitizesHTMLBody(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`<img src=x onerror=alert(1)>`,
		`<svg onload=alert(1)>`,
		`<iframe src="javascript:alert(1)"></iframe>`,
		`<a href="javascript:alert(1)">x</a>`,
		`<p onclick="alert(1)" style="x">x</p>`,
		`<style>body{background:url(javascript:alert(1))}</style>`,
		`<blockquote data-hitomado-quote><div data-hitomado-signature data-account-id="evil">x</div></blockquote>`,
	}
	for _, mode := range []model.Mode{model.ModeReply, model.ModeReplyAll, model.ModeForward} {
		for _, p := range payloads {
			s := src
			s.BodyHTML = "<p>ok</p>" + p
			out, _ := Build(mode, s)
			low := strings.ToLower(out)
			for _, bad := range []string{"<script", "<img", "<svg", "<iframe", "<style", "onerror", "onload", "onclick", "style=", "javascript:", "data-hitomado-signature", "alert(1)"} {
				assert.NotContains(t, low, bad, "mode=%s payload=%q out=%q", mode, p, out)
			}
			assert.Equal(t, 1, strings.Count(out, "data-hitomado-quote"), "引用マーカーは 1 つだけ: %q", out)
			assert.Contains(t, out, "<p>ok</p>")
		}
	}
}

func TestBuildEscapesHeaderAndPlainText(t *testing.T) {
	s := model.SourceMessage{
		From:     model.Address{Name: `<script>alert(1)</script>`, Email: `a"><img src=x onerror=alert(1)>@x.com`},
		Subject:  "<img src=x onerror=alert(1)>\r\nBcc: evil",
		To:       []model.Address{{Name: "<b>", Email: "t@x.com"}},
		BodyText: `<script>alert(1)</script>`,
	}
	for _, mode := range []model.Mode{model.ModeReply, model.ModeForward} {
		html, _ := Build(mode, s)
		low := strings.ToLower(html)
		assert.NotContains(t, low, "<script")
		assert.NotContains(t, low, "<img")
		assert.NotContains(t, low, "<b>")
		assert.NotContains(t, html, "\n")
	}
}

func TestBuildHTMLBodyThatSanitizesToNothingFallsBackToText(t *testing.T) {
	s := src
	s.BodyHTML = "<script>alert(1)</script>"
	s.BodyText = "plain"
	html, _ := Build(model.ModeReply, s)
	assert.Contains(t, html, "</p>plain</blockquote>")
}
