package platform

import "github.com/ousiassllc/hitomado/internal/notify"

// noticeText は通知の見出しと本文を返す。見出しはアカウント名、本文は差出人と件名（F-10）。
func noticeText(n notify.NewMailNotice) (title, body string) {
	title = n.AccountName
	from := n.FromName
	if from == "" {
		from = "(差出人不明)"
	}
	subject := n.Subject
	if subject == "" {
		subject = "(件名なし)"
	}
	return title, from + "\n" + subject
}
