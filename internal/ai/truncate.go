package ai

import "unicode/utf8"

// Truncate は msgs（時系列昇順）から、新しい方を優先して直近 maxMessages 件・合計 maxChars ルーン以内に
// 収まる分を、時系列昇順のまま返す。古い方から落とす。最新の 1 件だけで maxChars を超える場合は、
// その本文の先頭 maxChars ルーンだけを残す。入力は変更せず、結果は空でも nil にしない。
func Truncate(msgs []ContextMessage, maxMessages, maxChars int) []ContextMessage {
	if maxMessages <= 0 || maxChars <= 0 || len(msgs) == 0 {
		return []ContextMessage{}
	}
	kept, remaining := 0, maxChars
	for i := len(msgs) - 1; i >= 0 && kept < maxMessages; i-- {
		n := utf8.RuneCountInString(msgs[i].Text)
		if n > remaining {
			break
		}
		remaining -= n
		kept++
	}
	if kept == 0 {
		newest := msgs[len(msgs)-1]
		newest.Text = firstRunes(newest.Text, maxChars)
		return []ContextMessage{newest}
	}
	out := make([]ContextMessage, kept)
	copy(out, msgs[len(msgs)-kept:])
	return out
}

func firstRunes(s string, n int) string {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}
