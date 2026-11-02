package appcore

import (
	"encoding/base64"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

var cidRef = regexp.MustCompile(`(?i)\bcid:([^"'\s()<>]+)`)

// imageMIME は data: URL に埋め込んでよい MIME（image/* のみ。パラメータ・空白・大文字は不可）。
var imageMIME = regexp.MustCompile(`^image/[a-z0-9.+-]+$`)

// MaxInlineDataBytes は 1 メッセージで data: URL に置換する base64 の合計上限。超えた分は置換せず cid: のまま残す
// （表示側で読み込めない＝ブロックされる）。
const MaxInlineDataBytes = 10 << 20

// InlineCIDImages は本文中の cid: 参照を、対応するインライン画像の data: URL に置換する。
// parts のキーは収集側で NormalizeCID 済み（再正規化すると %25 を二重デコードする）。本文の参照だけを同じ
// mimeparse.NormalizeCID（大文字小文字・山括弧・%エンコードを吸収）で正規化し、完全一致で引く。
// 同じ cid は base64 を 1 回だけ作って使い回す。置換後の data: URL の合計（参照ごとに数える）が
// MaxInlineDataBytes を超える分・画像以外の MIME・
// 対応する部品が無い参照はそのまま残す（フロントのサニタイズが落とす）。
func InlineCIDImages(body string, parts map[string]gmail.InlinePart) string {
	if len(parts) == 0 || !strings.Contains(strings.ToLower(body), "cid:") {
		return body
	}
	urls := map[string]string{} // 正規化 cid → data URL（"" は置換しない）
	used := 0
	return cidRef.ReplaceAllStringFunc(body, func(m string) string {
		id := mimeparse.NormalizeCID(m)
		u, seen := urls[id]
		if !seen {
			u = dataURL(parts[id])
			urls[id] = u
		}
		// 出力に含まれる分（参照のたびに本文へ埋め込まれる）を合計する。
		if u == "" || used+len(u) > MaxInlineDataBytes {
			return m
		}
		used += len(u)
		return u
	})
}

// dataURL は画像部品の data: URL を返す。画像でない・MIME が不正・データが空なら "" を返す。
func dataURL(p gmail.InlinePart) string {
	mime := strings.ToLower(p.MimeType)
	if len(p.Data) == 0 || !imageMIME.MatchString(mime) {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
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
