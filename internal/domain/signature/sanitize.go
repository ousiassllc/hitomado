package signature

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// allowedTags は Sanitize が残すタグ。a は href のみ、それ以外は属性を持たない。
var allowedTags = map[atom.Atom]bool{
	atom.P: true, atom.Br: true, atom.Strong: true, atom.B: true, atom.Em: true, atom.I: true,
	atom.U: true, atom.A: true, atom.Ul: true, atom.Ol: true, atom.Li: true,
}

// dropWithContent は中身ごと捨てるタグ（テキストとして表示されては困るもの）。
var dropWithContent = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Head: true, atom.Title: true, atom.Template: true,
	atom.Noscript: true, atom.Iframe: true, atom.Object: true, atom.Embed: true, atom.Svg: true,
	atom.Math: true, atom.Textarea: true, atom.Select: true, atom.Button: true, atom.Applet: true,
	atom.Frame: true, atom.Frameset: true, atom.Noembed: true, atom.Noframes: true, atom.Xmp: true,
	atom.Plaintext: true, atom.Audio: true, atom.Video: true, atom.Canvas: true,
}

// blockTags は許可外だが、除去後に改行が失われると文が連結してしまうブロック要素。
// 中身を残し、直後に <br> を 1 つ入れる。
var blockTags = map[atom.Atom]bool{
	atom.Div: true, atom.Blockquote: true, atom.Pre: true, atom.Hr: true, atom.Tr: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true, atom.Table: true,
	atom.Dl: true, atom.Dt: true, atom.Dd: true, atom.Address: true, atom.Center: true,
	atom.Figure: true, atom.Figcaption: true, atom.Main: true, atom.Nav: true, atom.Aside: true,
	atom.Form: true, atom.Fieldset: true,
}

// Sanitize は許可タグ（p, br, strong, b, em, i, u, a[href], ul, ol, li）だけを残した HTML を返す。
// パースして再構築するため、属性（style・on* など）は a の href 以外すべて落ちる。
// href は http／https／mailto のみ許可し、それ以外のリンクは中身のテキストだけを残す。
// 許可外のタグは外して中身を残す（script 等は中身ごと捨てる）。入力が空白だけなら "" を返す。
func Sanitize(src string) string {
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{
		Type: html.ElementNode, DataAtom: atom.Body, Data: "body",
	})
	if err != nil {
		return html.EscapeString(src)
	}
	out := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	for _, n := range nodes {
		clean(out, n)
	}
	var b strings.Builder
	for c := out.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return html.EscapeString(src)
		}
	}
	return strings.TrimSpace(b.String())
}

// clean は n を許可規則で変換し、結果を dst の子として追加する。
func clean(dst, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		dst.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
	case html.ElementNode:
		cleanElement(dst, n)
	case html.DocumentNode:
		cleanChildren(dst, n)
	}
	// コメント・doctype・処理命令は捨てる
}

func cleanChildren(dst, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		clean(dst, c)
	}
}

func cleanElement(dst, n *html.Node) {
	switch {
	case dropWithContent[n.DataAtom]:
		return
	case n.DataAtom == atom.A:
		href, ok := safeHref(attr(n, "href"))
		if !ok {
			cleanChildren(dst, n)
			return
		}
		el := &html.Node{Type: html.ElementNode, DataAtom: atom.A, Data: "a",
			Attr: []html.Attribute{{Key: "href", Val: href}}}
		cleanChildren(el, n)
		dst.AppendChild(el)
	case allowedTags[n.DataAtom]:
		el := &html.Node{Type: html.ElementNode, DataAtom: n.DataAtom, Data: n.Data}
		cleanChildren(el, n)
		dst.AppendChild(el)
	case blockTags[n.DataAtom]:
		cleanChildren(dst, n)
		appendBreak(dst)
	default:
		cleanChildren(dst, n)
	}
}

// appendBreak は dst の末尾が改行でなければ <br> を追加する（先頭には追加しない）。
func appendBreak(dst *html.Node) {
	last := dst.LastChild
	if last == nil || (last.Type == html.ElementNode && (last.DataAtom == atom.Br || last.DataAtom == atom.P)) {
		return
	}
	dst.AppendChild(&html.Node{Type: html.ElementNode, DataAtom: atom.Br, Data: "br"})
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// safeHref は href が http／https／mailto の絶対 URL であれば、空白・制御文字を除いた値を返す。
// ブラウザは URL 中のタブ・改行を無視する（"java\tscript:" 等）ため、検査前に取り除く。
func safeHref(raw string) (string, bool) {
	cleaned := strings.Map(func(r rune) rune {
		if r <= 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, raw)
	u, err := url.Parse(cleaned)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto":
		if u.Opaque == "" && u.Path == "" {
			return "", false
		}
	default:
		return "", false
	}
	return cleaned, true
}
