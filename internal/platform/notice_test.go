package platform

import (
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
