package appcore_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

func png(data string) gmail.InlinePart {
	return gmail.InlinePart{MimeType: "image/png", Data: []byte(data)}
}

func TestInlineCIDImagesMatchesNormalizedCID(t *testing.T) {
	parts := map[string]gmail.InlinePart{"logo@example.com": png("AAA")} // 収集側は NormalizeCID 済み（小文字・%デコード済み）
	want := "data:image/png;base64,QUFB"
	for _, ref := range []string{"cid:logo@example.com", "CID:LOGO@Example.COM", "cid:logo%40example.com", "cid:Logo%40EXAMPLE.com"} {
		got := appcore.InlineCIDImages(`<img src="`+ref+`">`, parts)
		assert.Equal(t, `<img src="`+want+`">`, got, ref)
	}
	// 完全一致のみ: 前方一致・部分一致では置換しない。
	for _, ref := range []string{"cid:logo@example.co", "cid:logo@example.comx", "cid:other"} {
		body := `<img src="` + ref + `">`
		assert.Equal(t, body, appcore.InlineCIDImages(body, parts), ref)
	}
}

func TestInlineCIDImagesSameCIDMultipleReferences(t *testing.T) {
	parts := map[string]gmail.InlinePart{"a": png("AAA"), "b": png("BBB")}
	body := `<img src="cid:a"><img src='cid:A'><div style="background:url(cid:b)"></div><img src="cid:a">`
	got := appcore.InlineCIDImages(body, parts)
	assert.Equal(t, 3, strings.Count(got, "data:image/png;base64,QUFB"))
	assert.Equal(t, 1, strings.Count(got, "data:image/png;base64,QkJC"))
	assert.NotContains(t, got, "cid:")
}

func TestInlineCIDImagesRejectsNonImageOrOddMIME(t *testing.T) {
	bad := []string{"text/html", "application/pdf", "image/", "image/png; charset=x", "image/png\r\nX: y", "imagex/png", "", "image/p ng", "image/png,evil"}
	for _, mime := range bad {
		parts := map[string]gmail.InlinePart{"a": {MimeType: mime, Data: []byte("x")}}
		body := `<img src="cid:a">`
		assert.Equal(t, body, appcore.InlineCIDImages(body, parts), "MIME %q は置換しない", mime)
	}
	got := appcore.InlineCIDImages(`<img src="cid:a">`, map[string]gmail.InlinePart{"a": {MimeType: "IMAGE/SVG+xml", Data: []byte("x")}})
	assert.Equal(t, `<img src="data:image/svg+xml;base64,eA==">`, got, "大文字は小文字化して許可")
	body := `<img src="cid:a">`
	assert.Equal(t, body, appcore.InlineCIDImages(body, map[string]gmail.InlinePart{"a": {MimeType: "image/png"}}), "データが空なら置換しない")
	assert.Equal(t, body, appcore.InlineCIDImages(body, nil))
	assert.Equal(t, "plain", appcore.InlineCIDImages("plain", map[string]gmail.InlinePart{"a": png("x")}))
}

func TestInlineCIDImagesTotalLimit(t *testing.T) {
	// base64 後 ≒ 7MB の画像: 1 つ目の参照は置換され、合計が 10MiB を超える以降の参照は cid: のまま残る。
	big := gmail.InlinePart{MimeType: "image/jpeg", Data: make([]byte, 5<<20)}
	parts := map[string]gmail.InlinePart{"one": big, "two": big, "small": png("AAA")}
	got := appcore.InlineCIDImages(`<img src="cid:one"><img src="cid:two"><img src="cid:one"><img src="cid:small">`, parts)
	assert.Equal(t, 1, strings.Count(got, "data:image/jpeg;base64,"))
	assert.Equal(t, 1, strings.Count(got, `src="cid:two"`))
	assert.Equal(t, 1, strings.Count(got, `src="cid:one"`))
	assert.Contains(t, got, "data:image/png;base64,QUFB", "上限内に収まる小さい画像は置換される")
	assert.LessOrEqual(t, len(got), appcore.MaxInlineDataBytes+1024)
}

func TestHasRemoteImages(t *testing.T) {
	cases := map[string]bool{
		`<p>text</p>`:                                                              false,
		`<img src="https://t.example/p.gif">`:                                      true,
		`<img src="HTTP://t.example/p.gif">`:                                       true,
		`<img src="//t.example/p.gif">`:                                            true,
		`<img src="data:image/png;base64,AA==">`:                                   false,
		`<img src="cid:a">`:                                                        false,
		`<img srcset="a.png 1x, https://t.example/b.png 2x">`:                      true,
		`<table background="https://t.example/bg.png"><tr><td>x</td></tr></table>`: true,
		`<div style="background:url('https://t.example/x.png')"></div>`:            true,
		`<div style="background:url(data:image/png;base64,AA==)"></div>`:           false,
		`<style>.a{background:url(//t.example/x.png)}</style><p>x</p>`:             true,
		`<a href="https://t.example/">link</a>`:                                    false,
	}
	for body, want := range cases {
		assert.Equal(t, want, appcore.HasRemoteImages(body), body)
	}
	require.False(t, appcore.HasRemoteImages(""))
}
