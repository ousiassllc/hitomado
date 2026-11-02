package platform

import (
	"strings"
	"unicode"

	"github.com/ousiassllc/hitomado/internal/notify"
)

// maxNoticeRunes はトーストの 1 行あたりの上限。長い件名で通知が崩れないようにする。
const maxNoticeRunes = 120

// plainText は通知に載せる文字列から制御文字を除き、シェルの展開に使われる `$` と “ ` “ を無害な文字に置き換えて
// 長さを切り詰める。外部入力（件名・差出人名）がスクリプトとして解釈される経路への多層防御。
func plainText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '$':
			b.WriteRune('＄')
		case r == '`':
			b.WriteRune('\u2018')
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

// noticeText は通知の見出しと本文を返す。見出しはアカウント名、本文は差出人と件名（F-10）。
func noticeText(n notify.NewMailNotice) (title, body string) {
	title = plainText(n.AccountName)
	from := plainText(n.FromName)
	if from == "" {
		from = "(差出人不明)"
	}
	subject := plainText(n.Subject)
	if subject == "" {
		subject = "(件名なし)"
	}
	return title, from + "\n" + subject
}
