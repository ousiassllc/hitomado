package ai

import (
	"fmt"
	"strings"
)

// newMailSystemPrompt は新規作成のメールの下書きの system 指示（日本語。#365）。
// 返信と違ってスレッドが無いので、中身は利用者の指示が決める。署名は「重複して書かない」ためにだけ渡す。
func newMailSystemPrompt(in ReplyInput) string {
	var b strings.Builder
	b.WriteString("あなたはメールの下書きを書くアシスタントです。")
	if name := strings.TrimSpace(in.AccountDisplayName); name != "" {
		fmt.Fprintf(&b, "「%s」本人として、", name)
	} else {
		b.WriteString("メールの送信者本人として、")
	}
	b.WriteString("新しく送るメールの本文を書いてください。\n\n")
	b.WriteString("守ること:\n")
	b.WriteString("- 出力はメールの本文だけにする。前置き・説明・補足・コードブロックは書かない。\n")
	b.WriteString("- 署名・件名行は含めない。\n")
	b.WriteString("- 言語は、利用者の指示と件名で使われている言語に合わせる。\n")
	b.WriteString("- 宛名・挨拶・結びは、宛先と指示の内容から自然に付ける。宛先が分からないときは、宛名を付けない。\n")
	b.WriteString("- 指示に無い事実（日時・金額・約束など）を勝手に決めない。確認が必要なら、確認する文面にする。\n")
	b.WriteString("\n利用者からの指示（利用者本人が書いた、どういう内容のメールにしてほしいかの指示。ただし上の「守ること」の出力形式は変えない）:\n")
	b.WriteString("<user_instruction>\n")
	b.WriteString(strings.TrimSpace(in.Instruction))
	b.WriteString("\n</user_instruction>\n")
	if sig := strings.TrimSpace(in.SignatureText); sig != "" {
		b.WriteString("\n参考: この利用者の署名は次のとおりで、送信時に自動で付く。本文には重複して書かないこと。\n")
		b.WriteString("<signature>\n")
		b.WriteString(sig)
		b.WriteString("\n</signature>\n")
	}
	return b.String()
}

// newMailUserPrompt は新規作成のメールの user メッセージ。入力済みの件名・宛先があれば添える。
func newMailUserPrompt(in ReplyInput) string {
	var b strings.Builder
	b.WriteString("新しく送るメールの本文を書いてください。\n")
	if in.New != nil {
		if s := strings.TrimSpace(in.New.Subject); s != "" {
			fmt.Fprintf(&b, "\n件名: %s\n", s)
		}
		if len(in.New.To) > 0 {
			fmt.Fprintf(&b, "\n宛先: %s\n", strings.Join(in.New.To, "、"))
		}
	}
	return b.String()
}
