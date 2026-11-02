package appcore

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

var cidRef = regexp.MustCompile(`(?i)\bcid:([^"'\s)<>]+)`)

// InlineCIDImages は本文中の cid: 参照を、対応するインライン画像の data: URL に置換する。
// 画像以外の MIME・対応する部品が無い参照はそのまま残す（フロントのサニタイズが落とす）。
func InlineCIDImages(body string, parts map[string]gmail.InlinePart) string {
	if len(parts) == 0 || !strings.Contains(strings.ToLower(body), "cid:") {
		return body
	}
	return cidRef.ReplaceAllStringFunc(body, func(m string) string {
		id := m[len("cid:"):]
		part, ok := parts[id]
		if !ok {
			if dec, err := url.PathUnescape(id); err == nil {
				part, ok = parts[dec]
			}
		}
		if !ok || !strings.HasPrefix(strings.ToLower(part.MimeType), "image/") {
			return m
		}
		return "data:" + part.MimeType + ";base64," + base64.StdEncoding.EncodeToString(part.Data)
	})
}

var cssURL = regexp.MustCompile(`(?i)url\(\s*["']?\s*(https?:)?//`)

// HasRemoteImages は本文が外部の画像（http／https。プロトコル相対含む）を参照しているかを返す
// （img の src・srcset、background 属性、style 属性・style 要素の CSS url()）。
func HasRemoteImages(body string) bool {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return false
	}
	var walk func(n *html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && elementHasRemote(n) {
			return true
		}
		if n.Type == html.TextNode && n.Parent != nil && n.Parent.Data == "style" && cssURL.MatchString(n.Data) {
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	return walk(root)
}

func elementHasRemote(n *html.Node) bool {
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "src":
			if n.Data == "img" && isRemoteURL(a.Val) {
				return true
			}
		case "srcset":
			for _, cand := range strings.Split(a.Val, ",") {
				if f := strings.Fields(cand); len(f) > 0 && isRemoteURL(f[0]) {
					return true
				}
			}
		case "background":
			if isRemoteURL(a.Val) {
				return true
			}
		case "style":
			if cssURL.MatchString(a.Val) {
				return true
			}
		}
	}
	return false
}

func isRemoteURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "//")
}
