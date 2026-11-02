// Package mimeparse は Gmail API の MessagePart／ヘッダを解釈する（RFC 2047・アドレス・MIME ツリー）。
package mimeparse

import (
	"io"
	"mime"
	"net/mail"
	"strings"

	"golang.org/x/net/html/charset"
	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// wordDecoder は RFC 2047 のエンコード語を、ISO-2022-JP／Shift_JIS 等も含めてデコードする。
var wordDecoder = &mime.WordDecoder{CharsetReader: charsetReader}

var addressParser = &mail.AddressParser{WordDecoder: wordDecoder}

func charsetReader(label string, input io.Reader) (io.Reader, error) {
	return charset.NewReaderLabel(label, input)
}

// HeaderValue はヘッダ名（大文字小文字を区別しない）の最初の値を返す。
func HeaderValue(headers []*gmailapi.MessagePartHeader, name string) string {
	for _, h := range headers {
		if h != nil && strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// DecodeHeader は RFC 2047 エンコード語をデコードする。失敗したら元の文字列を返す。
func DecodeHeader(v string) string {
	if d, err := wordDecoder.DecodeHeader(v); err == nil {
		return d
	}
	return v
}

// ParseAddressList はアドレスヘッダを解析する（表示名は RFC 2047 デコード、Email は小文字）。
// 解析できない要素は読み飛ばす。
func ParseAddressList(v string) []model.Address {
	out := []model.Address{}
	v = strings.TrimSpace(v)
	if v == "" {
		return out
	}
	if list, err := addressParser.ParseList(v); err == nil {
		for _, a := range list {
			out = append(out, toAddress(a))
		}
		return out
	}
	// 一部が壊れたヘッダは要素ごとに救済する。
	for _, piece := range splitOutsideQuotes(v) {
		if a, err := addressParser.Parse(strings.TrimSpace(piece)); err == nil {
			out = append(out, toAddress(a))
		}
	}
	return out
}

func toAddress(a *mail.Address) model.Address {
	return model.Address{Name: a.Name, Email: strings.ToLower(a.Address)}
}

// ParseFrom は From ヘッダの先頭アドレスを返す（無ければゼロ値）。
func ParseFrom(v string) model.Address {
	if list := ParseAddressList(v); len(list) > 0 {
		return list[0]
	}
	return model.Address{}
}

// SplitFields は References のような空白区切りの Message-ID 列を分割する。
func SplitFields(v string) []string {
	out := []string{}
	out = append(out, strings.Fields(v)...)
	return out
}

// splitOutsideQuotes はアドレスヘッダをクォート（"…"）の外のカンマで分割する。
func splitOutsideQuotes(v string) []string {
	var out []string
	start, quoted := 0, false
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '\\':
			i++ // クォート内のバックスラッシュエスケープ
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				out = append(out, v[start:i])
				start = i + 1
			}
		}
	}
	return append(out, v[start:])
}
