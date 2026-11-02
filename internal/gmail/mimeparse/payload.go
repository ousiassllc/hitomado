package mimeparse

import (
	"encoding/base64"
	"io"
	"mime"
	"strings"

	"golang.org/x/net/html/charset"
	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// MaxInlineImageBytes はインライン画像 1 枚あたりの取得上限（超えたらスキップ）。
const MaxInlineImageBytes = 5 << 20

// Segment は本文パート 1 つ分。Data が空で AttID があるときは attachments.get で取得する。
type Segment struct {
	Data    string // base64url（API が返した形のまま）
	AttID   string
	Charset string
}

// InlineRef は本文から参照され得るインライン画像（Content-ID 付き画像）。
type InlineRef struct {
	CID      string
	MimeType string
	Data     string // base64url。空なら AttID から取得
	AttID    string
	Size     int64
	Info     model.AttachmentInfo
	AsAttach bool // Content-Disposition: attachment（本文から参照されなくても添付として出す）
}

// Parts は MIME ツリーを辿った結果（I/O 前の状態）。
type Parts struct {
	HTML, Text  []Segment
	Attachments []model.AttachmentInfo
	Inlines     []InlineRef
}

// Walk は MIME パートを再帰的に辿り、本文・添付・インライン画像の候補を集める。
func Walk(p *gmailapi.MessagePart) Parts {
	var out Parts
	walk(p, &out)
	return out
}

func walk(p *gmailapi.MessagePart, out *Parts) {
	if p == nil {
		return
	}
	mt := strings.ToLower(p.MimeType)
	if strings.HasPrefix(mt, "multipart/") {
		for _, c := range p.Parts {
			walk(c, out)
		}
		return
	}
	var data, attID string
	var size int64
	if p.Body != nil {
		data, attID, size = p.Body.Data, p.Body.AttachmentId, p.Body.Size
	}
	disposition := dispositionOf(p.Headers)
	if (mt == "text/html" || mt == "text/plain") && p.Filename == "" && disposition != "attachment" {
		seg := Segment{Data: data, AttID: attID, Charset: charsetOf(p.Headers)}
		if mt == "text/html" {
			out.HTML = append(out.HTML, seg)
		} else {
			out.Text = append(out.Text, seg)
		}
		return
	}
	info := model.AttachmentInfo{ID: attID, Filename: p.Filename, MimeType: p.MimeType, Size: size}
	if cid := contentID(p.Headers); cid != "" && strings.HasPrefix(mt, "image/") {
		out.Inlines = append(out.Inlines, InlineRef{
			CID: cid, MimeType: p.MimeType, Data: data, AttID: attID, Size: size,
			Info: info, AsAttach: disposition == "attachment",
		})
		return
	}
	if attID != "" {
		out.Attachments = append(out.Attachments, info)
	}
}

// HasAttachment は format=metadata の payload から添付の有無を近似する（multipart/mixed または filename 付きパート）。
func HasAttachment(p *gmailapi.MessagePart) bool {
	if p == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(p.MimeType), "multipart/mixed") || p.Filename != "" {
		return true
	}
	for _, c := range p.Parts {
		if HasAttachment(c) {
			return true
		}
	}
	return false
}

// dispositionOf は Content-Disposition の種別（attachment／inline／空）を小文字で返す。
func dispositionOf(headers []*gmailapi.MessagePartHeader) string {
	v := HeaderValue(headers, "Content-Disposition")
	if v == "" {
		return ""
	}
	d, _, err := mime.ParseMediaType(v)
	if err != nil {
		d, _, _ = strings.Cut(v, ";")
	}
	return strings.ToLower(strings.TrimSpace(d))
}

// contentID は Content-ID を NormalizeCID で正規化した値を返す。
func contentID(headers []*gmailapi.MessagePartHeader) string {
	return NormalizeCID(HeaderValue(headers, "Content-ID"))
}

// charsetOf は Content-Type の charset パラメータを返す（無ければ空）。
func charsetOf(headers []*gmailapi.MessagePartHeader) string {
	_, params, err := mime.ParseMediaType(HeaderValue(headers, "Content-Type"))
	if err != nil {
		return ""
	}
	return params["charset"]
}

// DecodeBase64URL は Gmail の base64url（パディングの有無を問わない）をデコードする。
func DecodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// DecodeText はバイト列を宣言された文字コードから UTF-8 の文字列にする。
func DecodeText(b []byte, label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return strings.ToValidUTF8(string(b), "\uFFFD")
	}
	r, err := charset.NewReaderLabel(label, strings.NewReader(string(b)))
	if err != nil {
		return strings.ToValidUTF8(string(b), "\uFFFD")
	}
	converted, err := io.ReadAll(r)
	if err != nil {
		return strings.ToValidUTF8(string(b), "\uFFFD")
	}
	return strings.ToValidUTF8(string(converted), "\uFFFD")
}
