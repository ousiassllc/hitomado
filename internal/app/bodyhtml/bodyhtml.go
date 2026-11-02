// Package bodyhtml は作成・AI 下書きで使う本文 HTML の小さな変換（エスケープ整形・署名/引用の除去・内部マーカーの除去）を提供する。
package bodyhtml

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
)

var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n+`)

// TextToHTML はプレーンテキストを HTML エスケープして <p>／<br> に整形する（AI の生成結果・下書きの text/plain 用）。
func TextToHTML(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, para := range paragraphBreak.Split(text, -1) {
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(strings.TrimSpace(para)), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

// UserBodyHTML は本文 HTML から署名ブロック・引用ブロックを除いた「利用者が書いた部分」を返す。
func UserBodyHTML(bodyHTML string) string {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(bodyHTML), ctx)
	if err != nil {
		return bodyHTML
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		pruneMarked(n)
		if isMarked(n) {
			continue
		}
		if err := xhtml.Render(&buf, n); err != nil {
			return bodyHTML
		}
	}
	return buf.String()
}

func isMarked(n *xhtml.Node) bool {
	if n.Type != xhtml.ElementNode {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == signature.Marker || a.Key == signature.QuoteMarker {
			return true
		}
	}
	return false
}

func pruneMarked(n *xhtml.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if isMarked(c) {
			n.RemoveChild(c)
		} else {
			pruneMarked(c)
		}
		c = next
	}
}

// IsEmpty は署名・引用を除いた本文が実質空か（空白のみ）を返す。
func IsEmpty(bodyHTML string) bool {
	return strings.TrimSpace(textconv.HTMLToText(UserBodyHTML(bodyHTML))) == ""
}

// 内部用の属性。送信先に届く HTML には出さない（署名・引用の検出はエディタ内だけで使う）。
var internalAttrs = []string{signature.Marker, signature.QuoteMarker, "data-account-id"}

func isInternalAttr(key string) bool {
	for _, k := range internalAttrs {
		if strings.EqualFold(key, k) {
			return true
		}
	}
	return false
}

// StripMarkers は署名・引用の内部属性（data-hitomado-signature／data-hitomado-quote／data-account-id）を
// HTML から取り除く。ブロック要素自体と内容は残す。対象の属性を持たないタグは元のバイト列のまま出力する。
func StripMarkers(bodyHTML string) string {
	z := xhtml.NewTokenizer(strings.NewReader(bodyHTML))
	var out strings.Builder
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return out.String()
		}
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			out.Write(z.Raw())
			continue
		}
		tok := z.Token()
		kept := tok.Attr[:0:0]
		for _, a := range tok.Attr {
			if !isInternalAttr(a.Key) {
				kept = append(kept, a)
			}
		}
		if len(kept) == len(tok.Attr) {
			out.Write(z.Raw())
			continue
		}
		tok.Attr = kept
		out.WriteString(tok.String())
	}
}
