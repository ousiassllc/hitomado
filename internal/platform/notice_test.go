package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/notify"
)

func TestNoticeText(t *testing.T) {
	title, body := noticeText(notify.NewMailNotice{AccountName: "A社", FromName: "山田", Subject: "見積"})
	assert.Equal(t, "A社", title)
	assert.Equal(t, "山田\n見積", body)

	_, body = noticeText(notify.NewMailNotice{AccountName: "A社"})
	assert.Equal(t, "(差出人不明)\n(件名なし)", body, "空の差出人・件名は代替表記にする")
}

func TestNoticeTextNeutralizesShellExpansionAndControlChars(t *testing.T) {
	_, body := noticeText(notify.NewMailNotice{AccountName: "A", FromName: "x$(calc)", Subject: "a`b\nc\x00d"})
	assert.NotContains(t, body, "$")
	assert.NotContains(t, body, "`")
	assert.NotContains(t, body, "\x00")
	assert.Equal(t, "x＄(calc)\na‘b c d", body)
}

func TestNoticeTextTruncatesLongSubject(t *testing.T) {
	_, body := noticeText(notify.NewMailNotice{AccountName: "A", FromName: "f", Subject: strings.Repeat("あ", 500)})
	assert.Equal(t, "f\n"+strings.Repeat("あ", maxNoticeRunes)+"…", body)
}
