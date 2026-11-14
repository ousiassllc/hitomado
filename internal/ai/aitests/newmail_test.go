package aitests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/ai"
)

var newMailInput = ai.ReplyInput{
	AccountDisplayName: "山田 太郎",
	SignatureText:      "山田 太郎\nExample 株式会社",
	Instruction:        "  来週の打ち合わせの日程を相談したい  ",
	New: &ai.NewMailContext{
		Subject: "打ち合わせの日程",
		To:      []string{"Hanako <hanako@example.com>", "taro@example.com"},
	},
}

// 新規作成は、スレッドを前提にした文言を使わず、指示は system の専用ブロックに、件名・宛先は user メッセージに入る（#365）。署名は参考として system にだけ入る。
func TestNewMailPromptHasNoThreadAndKeepsTheOutputRules(t *testing.T) {
	for _, name := range promptProviders {
		system, user := promptsOf(t, name, newMailInput)
		assert.Contains(t, system, "山田 太郎", name)
		assert.Contains(t, system, "新しく送るメールの本文", name)
		assert.Contains(t, system, "メールの本文だけ", name)
		assert.Contains(t, system, "署名・件名行は含めない", name)
		assert.Contains(t, system, "<user_instruction>\n来週の打ち合わせの日程を相談したい\n</user_instruction>", name)
		assert.Contains(t, system, "重複して書かない", name)
		assert.Contains(t, system, "Example 株式会社", name)
		assert.NotContains(t, system, "スレッド", name)
		assert.NotContains(t, system, "返信", name)

		assert.Contains(t, user, "件名: 打ち合わせの日程", name)
		assert.Contains(t, user, "宛先: Hanako <hanako@example.com>、taro@example.com", name)
		assert.NotContains(t, user, "来週の打ち合わせ", name, "指示は user メッセージに混ぜない")
		assert.NotContains(t, user, "Example 株式会社", name)
		assert.NotContains(t, user, "メッセージ 1", name)
	}
}

// 件名・宛先・表示名・署名が無いときは、その行・ブロックごと省く。
func TestNewMailPromptOmitsWhatIsMissing(t *testing.T) {
	in := ai.ReplyInput{Instruction: "お礼を伝える", New: &ai.NewMailContext{}}
	for _, name := range promptProviders {
		system, user := promptsOf(t, name, in)
		assert.Contains(t, system, "メールの送信者本人として", name)
		assert.Contains(t, system, "<user_instruction>\nお礼を伝える\n</user_instruction>", name)
		assert.NotContains(t, system, "<signature>", name)
		assert.NotContains(t, user, "件名:", name)
		assert.NotContains(t, user, "宛先:", name)
	}
}
