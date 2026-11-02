// Package mimebuild は Gmail で送信・下書き保存する RFC 5322 メッセージ（生 MIME）を構築する。
// 公開 API は gmail.BuildMIME／gmail.BuildDraftMIME。
package mimebuild

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"
	"github.com/oklog/ulid/v2"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
)

// ErrInvalid は入力が不正（ヘッダインジェクション・不正なアドレス・宛先なし等）なことを表す。
var ErrInvalid = errors.New("gmail: invalid outgoing message")

// Build は送信メッセージを RFC 5322 の生 MIME にする（Gmail の raw にする前の形）。
// multipart/alternative（text/plain＋text/html）で、添付があれば multipart/mixed。
// ヘッダ値に CR/LF が含まれる入力はヘッダインジェクションとして拒否する。
func Build(m model.OutgoingMessage) ([]byte, error) {
	return buildMIME(m, true)
}

// BuildDraft は下書き用の Build。宛先が 1 件もなくてもよい（To ヘッダは出力しない）。
// それ以外の検証と出力は Build と同じ。
func BuildDraft(m model.OutgoingMessage) ([]byte, error) {
	return buildMIME(m, false)
}

func buildMIME(m model.OutgoingMessage, requireRecipients bool) ([]byte, error) {
	if err := validateOutgoing(m, requireRecipients); err != nil {
		return nil, err
	}
	// enmime は To／Cc のどちらも無いメッセージを組み立てられない（Bcc は宛先に数えない）。
	// その場合は仮の To で組み立て、出力前に To ヘッダごと取り除く。
	noVisibleTo := len(m.To)+len(m.Cc) == 0
	text := m.BodyText
	if text == "" {
		text = textconv.HTMLToText(m.BodyHTML)
	}
	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}
	messageID, sendID := newMessageID(m.From.Email)
	b := enmime.Builder().
		From(m.From.Name, m.From.Email).
		ToAddrs(toNetAddrs(m.To)).
		CCAddrs(toNetAddrs(m.Cc)).
		Subject(m.Subject).
		Date(date).
		Text([]byte(text)).
		HTML([]byte(m.BodyHTML)).
		Header("Message-ID", messageID).
		Header(SendIDHeader, sendID)
	if noVisibleTo {
		b = b.To("", m.From.Email)
	}
	if len(m.Bcc) > 0 {
		b = b.Header("Bcc", joinAddrs(m.Bcc))
	}
	if m.InReplyTo != "" {
		b = b.Header("In-Reply-To", m.InReplyTo)
	}
	if len(m.References) > 0 {
		b = b.Header("References", strings.Join(m.References, " "))
	}
	for _, a := range m.Attachments {
		mt := a.MimeType
		if mt == "" {
			mt = "application/octet-stream"
		}
		b = b.AddAttachment(a.Data, mt, a.Filename)
	}
	root, err := b.Build()
	if err != nil {
		return nil, fmt.Errorf("%w: MIME の構築に失敗しました", ErrInvalid)
	}
	if noVisibleTo {
		root.Header.Del("To")
	}
	var buf bytes.Buffer
	if err := root.Encode(&buf); err != nil {
		return nil, fmt.Errorf("%w: MIME のエンコードに失敗しました", ErrInvalid)
	}
	return buf.Bytes(), nil
}

// SendIDHeader は送信ごとの識別子（ULID）を入れる独自ヘッダ名。
// Gmail API は送信時に Message-ID を付け替えることがあるため、messages.send の応答を受け取れなかったとき
// 送信済みかを確かめる手掛かりには、書き換えられにくいこのヘッダも使う（gmail.Client.Send）。
const SendIDHeader = "X-Hitomado-Send-Id"

// newMessageID は <ULID@From のドメイン> 形式の Message-ID と、同じ ULID の送信識別子を作る。
func newMessageID(from string) (messageID, sendID string) {
	domain := from[strings.LastIndex(from, "@")+1:]
	sendID = ulid.Make().String()
	return "<" + sendID + "@" + domain + ">", sendID
}

func validateOutgoing(m model.OutgoingMessage, requireRecipients bool) error {
	if m.From.Email == "" {
		return fmt.Errorf("%w: 送信元がありません", ErrInvalid)
	}
	if requireRecipients && len(m.To)+len(m.Cc)+len(m.Bcc) == 0 {
		return fmt.Errorf("%w: 宛先がありません", ErrInvalid)
	}
	all := append(append(append([]model.Address{m.From}, m.To...), m.Cc...), m.Bcc...)
	for _, a := range all {
		if hasLineBreak(a.Name) || hasLineBreak(a.Email) {
			return fmt.Errorf("%w: アドレスに改行が含まれています", ErrInvalid)
		}
		if p, err := mail.ParseAddress(a.Email); err != nil || p.Address != a.Email {
			return fmt.Errorf("%w: メールアドレスの形式が不正です", ErrInvalid)
		}
	}
	headers := append([]string{m.Subject, m.InReplyTo}, m.References...)
	for _, v := range headers {
		if hasLineBreak(v) {
			return fmt.Errorf("%w: ヘッダに改行が含まれています", ErrInvalid)
		}
	}
	for _, a := range m.Attachments {
		if hasLineBreak(a.Filename) || hasLineBreak(a.MimeType) {
			return fmt.Errorf("%w: 添付のヘッダに改行が含まれています", ErrInvalid)
		}
		if a.MimeType != "" {
			if _, _, err := mime.ParseMediaType(a.MimeType); err != nil {
				return fmt.Errorf("%w: 添付の MIME タイプが不正です", ErrInvalid)
			}
		}
	}
	return nil
}

func hasLineBreak(s string) bool { return strings.ContainsAny(s, "\r\n") }

func toNetAddrs(in []model.Address) []mail.Address {
	out := make([]mail.Address, 0, len(in))
	for _, a := range in {
		out = append(out, mail.Address{Name: a.Name, Address: a.Email})
	}
	return out
}

// joinAddrs はアドレス列を RFC 5322 のヘッダ値にする（表示名は RFC 2047 でエンコードされる）。
func joinAddrs(in []model.Address) string {
	parts := make([]string, 0, len(in))
	for _, a := range toNetAddrs(in) {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ", ")
}
