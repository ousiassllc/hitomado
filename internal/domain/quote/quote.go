// Package quote は返信・転送の引用本文（HTML／テキスト）を生成する（F-05。純粋ロジック）。
//
// 元メッセージの HTML はそのまま埋め込まず signature.Sanitize を通す。
// プレーンテキストしか無い場合はエスケープして <br> で改行する。
package quote

import (
	"html"
	"strings"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
)

// dateLayout は引用ヘッダの日時書式。src.Date のタイムゾーンはそのまま使う。
const dateLayout = "2006-01-02 15:04"

// forwardRule は転送ヘッダの先頭行。
const forwardRule = "---------- 転送されたメッセージ ----------"

// Build は mode の引用本文を返す。HTML は `<blockquote data-hitomado-quote>…</blockquote>`。
//   - reply／reply_all: 「日時 差出人 が書きました:」の 1 行＋元本文。テキスト版は各行に "> " を付ける
//   - forward: 差出人・日時・件名・宛先（・Cc）のヘッダ＋元本文
//   - new／その他: 空
func Build(mode model.Mode, src model.SourceMessage) (htmlOut, text string) {
	var head []string
	switch mode {
	case model.ModeReply, model.ModeReplyAll:
		head = []string{attribution(src)}
	case model.ModeForward:
		head = forwardHeader(src)
	default:
		return "", ""
	}

	bodyHTML, bodyText := body(src)
	var b strings.Builder
	b.WriteString("<blockquote " + signature.QuoteMarker + "><p>")
	for i, line := range head {
		if i > 0 {
			b.WriteString("<br>")
		}
		b.WriteString(html.EscapeString(line))
	}
	b.WriteString("</p>" + bodyHTML + "</blockquote>")

	if mode == model.ModeForward {
		return b.String(), strings.Join(head, "\n") + "\n\n" + bodyText
	}
	return b.String(), strings.Join(head, "\n") + "\n" + prefixLines(bodyText)
}

// attribution は返信の引用見出し（例: "2026-01-02 15:04 Alice <alice@x.com> が書きました:"）。
func attribution(src model.SourceMessage) string {
	parts := []string{}
	if d := formatDate(src); d != "" {
		parts = append(parts, d)
	}
	parts = append(parts, formatAddress(src.From)+" が書きました:")
	return strings.Join(parts, " ")
}

func forwardHeader(src model.SourceMessage) []string {
	head := []string{forwardRule, "差出人: " + formatAddress(src.From)}
	if d := formatDate(src); d != "" {
		head = append(head, "日時: "+d)
	}
	head = append(head, "件名: "+oneLine(src.Subject))
	if len(src.To) > 0 {
		head = append(head, "宛先: "+formatAddresses(src.To))
	}
	if len(src.Cc) > 0 {
		head = append(head, "Cc: "+formatAddresses(src.Cc))
	}
	return head
}

// body は元本文の安全な HTML と、テキスト版を返す。
func body(src model.SourceMessage) (htmlOut, text string) {
	if strings.TrimSpace(src.BodyHTML) != "" {
		if safe := signature.Sanitize(src.BodyHTML); safe != "" {
			return safe, trimText(textOf(src))
		}
	}
	t := trimText(src.BodyText)
	return strings.ReplaceAll(html.EscapeString(t), "\n", "<br>"), t
}

// textOf はテキスト版の元本文。プレーンテキスト部があればそれを、無ければ HTML から変換する。
func textOf(src model.SourceMessage) string {
	if strings.TrimSpace(src.BodyText) != "" {
		return src.BodyText
	}
	return textconv.HTMLToText(src.BodyHTML)
}

// trimText は改行コードを LF にそろえ、前後の空白行を除く。
func trimText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.TrimSpace(s)
}

func prefixLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

func formatDate(src model.SourceMessage) string {
	if src.Date.IsZero() {
		return ""
	}
	return src.Date.Format(dateLayout)
}

// formatAddress は "名前 <email>"（名前が無ければ email のみ）。ヘッダ行を崩す改行は空白にする。
func formatAddress(a model.Address) string {
	name, email := oneLine(a.Name), oneLine(a.Email)
	if name == "" || name == email {
		return email
	}
	return name + " <" + email + ">"
}

func formatAddresses(as []model.Address) string {
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = formatAddress(a)
	}
	return strings.Join(parts, ", ")
}

// oneLine は改行・タブを空白 1 つにして前後を除く。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
