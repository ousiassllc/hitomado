package notify

import (
	"strings"
	"unicode"
)

// maxNoticeRunes はトーストの 1 行あたりの上限。長い件名で通知が崩れないようにする。
const maxNoticeRunes = 120

// PlainText は通知に載せる文字列から制御文字（空白に置換）と不可視の書式文字（除去）を取り除き、長さを切り詰める。
// 外部入力（件名・差出人名）による表示の偽装・崩れへの対策。
func PlainText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cf, r): // 双方向制御などの不可視文字は捨てる（差出人名の偽装対策）
		case unicode.IsControl(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(out); len(r) > maxNoticeRunes {
		return string(r[:maxNoticeRunes]) + "…"
	}
	return out
}

// Text は通知の見出しと本文を返す。見出しはアカウント名、本文は差出人と件名（F-10）。
// Windows のトーストも Android の通知（Java へ渡す前）も同じ文字列を使う。
func Text(n NewMailNotice) (title, body string) {
	title = PlainText(n.AccountName)
	from := PlainText(n.FromName)
	if from == "" {
		from = "(差出人不明)"
	}
	subject := PlainText(n.Subject)
	if subject == "" {
		subject = "(件名なし)"
	}
	return title, from + "\n" + subject
}
