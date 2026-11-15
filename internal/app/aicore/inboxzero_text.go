package aicore

import (
	"path"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// Inbox Zero の判定で使う、メッセージ・文字列を扱う純粋な補助関数。

// classifyState は判定の AI へ送る内容（差出人・件名・メールの形の手がかり・本文の先頭 500 文字だけ）。Jev でもチャット系のモデルでも同じ。
// 手がかり（Content-Types・Attachments）は事実を並べるだけで、カレンダーかどうかは AI が決める（端末内では決めない）。
func classifyState(m gmail.MessageFull) string {
	var b strings.Builder
	b.WriteString("From: " + sender(m) + "\nSubject: " + m.Subject + "\n")
	if len(m.PartTypes) > 0 {
		b.WriteString("Content-Types: " + strings.Join(m.PartTypes, ", ") + "\n")
	}
	if a := attachmentHints(m); a != "" {
		b.WriteString("Attachments: " + a + "\n")
	}
	b.WriteString("\n" + firstRunes(messageText(m), inboxZeroBodyRunes))
	return b.String()
}

// attachmentHints は添付の種類と拡張子（".ics" など）をカンマ区切りで返す。ファイル名・サイズは AI へ送らない。
func attachmentHints(m gmail.MessageFull) string {
	hints := make([]string, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		h := strings.ToLower(a.MimeType)
		if ext := strings.ToLower(path.Ext(a.Filename)); ext != "" {
			h += " (" + ext + ")"
		}
		hints = append(hints, h)
	}
	return strings.Join(hints, ", ")
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
