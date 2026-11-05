// Package textconv は HTML からプレーンテキストへの変換を提供する（純粋ロジック）。
package textconv

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var blockTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Li: true, atom.Ul: true, atom.Ol: true, atom.Blockquote: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Tr: true, atom.Table: true, atom.Pre: true, atom.Hr: true,
}

// HTMLToText は HTML をプレーンテキストにする。ブロック要素は改行、<br> は改行、
// リンクは "text (url)"、script/style は除去する。連続する空行は 1 つに畳む。
func HTMLToText(src string) string {
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return strings.TrimSpace(src)
	}
	var b strings.Builder
	walk(&b, root)
	return tidy(b.String())
}

func walk(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
		return
	case html.ElementNode:
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Head, atom.Title:
			return
		case atom.Br:
			b.WriteString("\n")
			return
		case atom.Li:
			b.WriteString("\n- ")
		}
		if blockTags[n.DataAtom] && n.DataAtom != atom.Li {
			b.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(b, c)
		}
		if n.DataAtom == atom.A {
			if href := attr(n, "href"); href != "" && href != text(n) {
				b.WriteString(" (" + href + ")")
			}
		}
		if blockTags[n.DataAtom] && n.DataAtom != atom.Li {
			b.WriteString("\n")
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(b, c)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func text(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return strings.TrimSpace(b.String())
}

// tidy は行末空白を除き、3 行以上の連続改行を 2 行に畳み、前後の空白を除く。
func tidy(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Preview は HTML をテキスト化し、先頭 maxRunes ルーンに切り詰める（超過時は末尾に … を付ける）。
// 改行と段落の空行は保ち、各行の中の空白だけを 1 つに畳む。
func Preview(src string, maxRunes int) string {
	t := collapseLines(HTMLToText(src))
	if maxRunes <= 0 || utf8.RuneCountInString(t) <= maxRunes {
		return t
	}
	r := []rune(t)
	return strings.TrimRight(string(r[:maxRunes]), "\n ") + "…"
}

// collapseLines は各行の連続する空白を 1 つにし、空白だけの行は空行として、連続する空行は 1 つに畳む。
func collapseLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" && len(out) > 0 && out[len(out)-1] == "" {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
