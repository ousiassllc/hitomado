package aicore

import (
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// Inbox Zero の判定で使う、メッセージ・文字列を扱う純粋な補助関数。

// classifyState は判定の AI へ送る内容（差出人・件名・本文の先頭 500 文字だけ）。Jev でもチャット系のモデルでも同じ。
func classifyState(m gmail.MessageFull) string {
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

// isOwnMessage は m が自分の送信（SENT ラベル、または差出人が自分のアドレス。大文字小文字を区別しない）か。
func isOwnMessage(m gmail.MessageFull, email string) bool {
	return slices.Contains(m.LabelIDs, labelSent) || (email != "" && strings.EqualFold(strings.TrimSpace(m.From.Email), email))
}

// googleCalendarSender は Google カレンダーの通知（招待・変更・リマインダー・予定表のまとめ）の送信元アドレス。
// 招待・変更は iCalendar のパートを持つが、リマインダーや予定表のまとめは持たないので、送信元でも見る。
const googleCalendarSender = "calendar-notification@google.com"

// isCalendarMail は m がカレンダーの招待・通知か（iCalendar のパートを持つ、または Google カレンダーからの送信。端末内の判定）。
func isCalendarMail(m gmail.MessageFull) bool {
	return m.HasCalendar || strings.EqualFold(strings.TrimSpace(m.From.Email), googleCalendarSender)
}
