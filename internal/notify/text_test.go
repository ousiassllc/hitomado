package notify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoticeText(t *testing.T) {
	title, body := Text(NewMailNotice{AccountName: "A社", FromName: "山田", Subject: "見積"})
	assert.Equal(t, "A社", title)
	assert.Equal(t, "山田\n見積", body)

	_, body = Text(NewMailNotice{AccountName: "A社"})
	assert.Equal(t, "(差出人不明)\n(件名なし)", body, "空の差出人・件名は代替表記にする")
}

func TestNoticeTextStripsControlAndInvisibleChars(t *testing.T) {
	_, body := Text(NewMailNotice{AccountName: "A", FromName: "x$100\u202Eevil", Subject: "a\nb\x00c"})
	assert.Equal(t, "x$100evil\na b c", body, "制御文字・双方向制御文字は除き、$ などの通常文字は保つ")
}

func TestNoticeTextTruncatesLongSubject(t *testing.T) {
	_, body := Text(NewMailNotice{AccountName: "A", FromName: "f", Subject: strings.Repeat("あ", 500)})
	assert.Equal(t, "f\n"+strings.Repeat("あ", maxNoticeRunes)+"…", body)
}
