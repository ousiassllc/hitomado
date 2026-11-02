// Package signature はアカウント別署名ブロックの生成・挿入と HTML サニタイズを提供する（F-07）。
package signature

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Marker は署名ブロックの識別属性名。
const Marker = "data-hitomado-signature"

// QuoteMarker は引用ブロック（<blockquote>）の識別属性名（quote パッケージと共通）。
const QuoteMarker = "data-hitomado-quote"

const accountAttr = "data-account-id"

// Block は署名ブロック `<div data-hitomado-signature data-account-id="…">-- <br>署名</div>` を返す。
// 署名は Sanitize 済みのものだけを埋め込み、空（空白のみ・許可タグが残らない場合を含む）なら "" を返す。
func Block(accountID, sigHTML string) string {
	sig := Sanitize(sigHTML)
	if sig == "" {
		return ""
	}
	return "<div " + Marker + " " + accountAttr + `="` + html.EscapeString(accountID) + `">-- <br>` + sig + "</div>"
}

// Insert は bodyHTML に署名ブロックを挿入して返す。引用ブロック（data-hitomado-quote）の
// 直前に入れ、無ければ末尾に入れる。同じアカウントの署名ブロックがすでにあれば何もしない。
// 署名が空なら bodyHTML をそのまま返す。
func Insert(bodyHTML, accountID, sigHTML string) string {
	block := Block(accountID, sigHTML)
	if block == "" || hasBlock(bodyHTML, accountID) {
		return bodyHTML
	}
	if at, ok := findQuote(bodyHTML); ok {
		return bodyHTML[:at] + block + bodyHTML[at:]
	}
	return bodyHTML + block
}

// hasBlock は accountID の署名ブロックが bodyHTML に含まれるかを返す。
func hasBlock(bodyHTML, accountID string) bool {
	z := xhtml.NewTokenizer(strings.NewReader(bodyHTML))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return false
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			_, attrs := readTag(z)
			if _, ok := attrs[Marker]; ok && attrs[accountAttr] == accountID {
				return true
			}
		}
	}
}

// findQuote は最初の引用ブロックの開始タグのバイト位置を返す。
func findQuote(bodyHTML string) (int, bool) {
	z := xhtml.NewTokenizer(strings.NewReader(bodyHTML))
	offset := 0
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return 0, false
		}
		// TagName／TagAttr は Raw のバッファを書き換えうるため、先に長さを控える。
		n := len(z.Raw())
		if tt == xhtml.StartTagToken {
			if name, attrs := readTag(z); name == "blockquote" {
				if _, ok := attrs[QuoteMarker]; ok {
					return offset, true
				}
			}
		}
		offset += n
	}
}

// readTag は現在の開始タグの名前（小文字）と属性を返す。
func readTag(z *xhtml.Tokenizer) (string, map[string]string) {
	name, has := z.TagName()
	attrs := map[string]string{}
	for has {
		var k, v []byte
		k, v, has = z.TagAttr()
		attrs[string(k)] = string(v)
	}
	return string(name), attrs
}
