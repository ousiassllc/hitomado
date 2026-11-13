package aicore

import (
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// Inbox Zero の判定で使う、メッセージ・文字列を扱う純粋な補助関数。

// jevState は TypeSafe AI へ送る内容（差出人・件名・本文の先頭 500 文字だけ）。
func jevState(m gmail.MessageFull) string {
	return "From: " + sender(m) + "\nSubject: " + m.Subject + "\n\n" + firstRunes(messageText(m), inboxZeroBodyRunes)
}

// sender は差出人を「表示名 <アドレス>」（表示名が無ければアドレス）にする。
func sender(m gmail.MessageFull) string {
	if m.From.Name == "" {
		return m.From.Email
	}
	return m.From.Name + " <" + m.From.Email + ">"
}

// firstRunes は s の先頭 n 文字（rune）。
func firstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// nonDraftMessages はスレッドの下書きでないメッセージ（古い順）。
func nonDraftMessages(thread gmail.ThreadFull) []gmail.MessageFull {
	return slices.DeleteFunc(slices.Clone(thread.Messages), func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, labelDraft) })
}

// hasOwnMessage はメッセージに自分が送ったもの（SENT ラベル、または差出人が自分のアドレス。大文字小文字を区別しない）があるか。
func hasOwnMessage(msgs []gmail.MessageFull, email string) bool {
	return slices.ContainsFunc(msgs, func(m gmail.MessageFull) bool {
		return slices.Contains(m.LabelIDs, labelSent) || (email != "" && strings.EqualFold(strings.TrimSpace(m.From.Email), email))
	})
}
