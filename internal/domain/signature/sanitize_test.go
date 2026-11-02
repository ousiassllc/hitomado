package signature

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"空", "", ""},
		{"空白のみ", " \n\t ", ""},
		{"プレーンテキスト", "hello", "hello"},
		{"テキストはエスケープ", "a &lt;b&gt; &amp; c", "a &lt;b&gt; &amp; c"},
		{"許可タグを保持", "<p>a<br><strong>b</strong><b>c</b><em>d</em><i>e</i><u>f</u></p>",
			"<p>a<br/><strong>b</strong><b>c</b><em>d</em><i>e</i><u>f</u></p>"},
		{"リスト", "<ul><li>a</li><li>b</li></ul><ol><li>c</li></ol>",
			"<ul><li>a</li><li>b</li></ul><ol><li>c</li></ol>"},
		{"リンクは href のみ", `<a href="https://example.com/x?a=1&amp;b=2" target="_blank" onclick="x()" style="color:red">link</a>`,
			`<a href="https://example.com/x?a=1&amp;b=2">link</a>`},
		{"mailto", `<a href="mailto:a@example.com">m</a>`, `<a href="mailto:a@example.com">m</a>`},
		{"http", `<a href="http://example.com">h</a>`, `<a href="http://example.com">h</a>`},
		{"許可タグの属性は全削除", `<p class="x" style="x" onmouseover="alert(1)" id="y">t</p>`, "<p>t</p>"},
		{"許可外タグは外して中身を残す", `<span style="color:red">a</span><font color="red">b</font>`, "ab"},
		{"ブロック要素は改行になる", "<div>a</div><div>b</div>", "a<br/>b<br/>"},
		{"入れ子ブロックの改行は 1 つ", "<div><div>a</div></div>c", "a<br/>c"},
		{"見出し", "<h1>t</h1>x", "t<br/>x"},
		{"コメント除去", "a<!-- <script>alert(1)</script> -->b", "ab"},
		{"属性値の中のタグ風文字列", `<p title="<script>alert(1)</script>">t</p>`, "<p>t</p>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Sanitize(tt.in))
		})
	}
}

func TestSanitizeXSSPayloads(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`<SCRIPT SRC=//evil.example/x.js></SCRIPT>`,
		`<img src=x onerror=alert(1)>`,
		`<svg onload=alert(1)><script>alert(2)</script></svg>`,
		`<svg><script>alert(1)</script></svg>`,
		`<math><mtext><script>alert(1)</script></mtext></math>`,
		`<iframe src="javascript:alert(1)"></iframe>`,
		`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		`<object data="javascript:alert(1)"></object>`,
		`<embed src="javascript:alert(1)">`,
		`<body onload=alert(1)>`,
		`<style>@import 'javascript:alert(1)';</style>`,
		`<p style="background:url(javascript:alert(1))">x</p>`,
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<a href="  javascript:alert(1)">x</a>`,
		"<a href=\"java\tscript:alert(1)\">x</a>",
		"<a href=\"java\nscript:alert(1)\">x</a>",
		`<a href="&#106;avascript:alert(1)">x</a>`,
		`<a href="&#x6A;avascript&colon;alert(1)">x</a>`,
		`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
		`<a href="vbscript:msgbox(1)">x</a>`,
		`<a href="//evil.example/x">x</a>`,
		`<a href="/relative">x</a>`,
		`<a href="https:">x</a>`,
		`<form action="javascript:alert(1)"><button>go</button></form>`,
		`<input autofocus onfocus=alert(1)>`,
		`<details open ontoggle=alert(1)>`,
		`<video><source onerror=alert(1)></video>`,
		`<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript>`,
		`<template><script>alert(1)</script></template>`,
		`<textarea></textarea><script>alert(1)</script>`,
		`<<script>script>alert(1)</script>`,
		`<a href="https://example.com" onclick="alert(1)" onmouseover="alert(2)">x</a>`,
		`<base href="javascript:alert(1)//">`,
		`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		`<link rel=stylesheet href="javascript:alert(1)">`,
	}
	for _, p := range payloads {
		out := Sanitize(p)
		low := strings.ToLower(out)
		for _, bad := range []string{"<script", "<img", "<svg", "<iframe", "<object", "<embed", "<style", "<form",
			"<input", "<body", "<base", "<meta", "<link", "<math", "<video", "<details", "<template", "<textarea",
			"onerror", "onload", "onclick", "onfocus", "onmouseover", "ontoggle", "style=", "javascript:", "vbscript:", "data:"} {
			assert.NotContains(t, low, bad, "payload=%q out=%q", p, out)
		}
		assert.NotContains(t, low, "alert(1)", "script 本文が残っていないこと: payload=%q out=%q", p, out)
	}
}

func TestSanitizeOutputOnlyAllowedTags(t *testing.T) {
	out := Sanitize(`<div><h1>t</h1><table><tr><td>c</td></tr></table><p>p<a href="https://e.com">l</a></p></div>`)
	for _, tag := range []string{"<div", "<h1", "<table", "<tr", "<td"} {
		assert.NotContains(t, out, tag)
	}
	assert.Contains(t, out, `<a href="https://e.com">l</a>`)
}

func TestSanitizeIdempotent(t *testing.T) {
	in := `<div>a<br>b<a href="https://example.com/?q=1&amp;r=2" onclick="x">c</a><ul><li>d</li></ul><script>x</script></div>`
	once := Sanitize(in)
	assert.Equal(t, once, Sanitize(once))
}
