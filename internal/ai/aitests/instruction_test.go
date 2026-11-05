package aitests

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

const instructionBlockTitle = "利用者からの指示"

// promptsOf は provider に in を渡したときの system 指示と user メッセージ（本体の本文）を返す。
func promptsOf(t *testing.T, name string, in ai.ReplyInput) (system, user string) {
	t.Helper()
	switch name {
	case ai.ProviderAnthropic:
		srv, rec := serve(t, 200, `{"content":[{"type":"text","text":"ok"}]}`)
		_, err := provider(t, name, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
		msgs, _ := rec.body["messages"].([]any)
		require.Len(t, msgs, 1)
		return rec.body["system"].(string), msgs[0].(map[string]any)["content"].(string)
	case ai.ProviderOpenAI:
		srv, rec := serve(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
		_, err := provider(t, name, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
		msgs, _ := rec.body["messages"].([]any)
		require.Len(t, msgs, 2)
		return msgs[0].(map[string]any)["content"].(string), msgs[1].(map[string]any)["content"].(string)
	default:
		srv, rec := serve(t, 200, deltaEvent("ok")+completedEvent)
		_, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
		items, _ := rec.body["input"].([]any)
		require.Len(t, items, 1)
		return rec.body["instructions"].(string), items[0].(map[string]any)["content"].(string)
	}
}

var promptProviders = []string{ai.ProviderAnthropic, ai.ProviderOpenAI, ai.ProviderChatGPT}

// 指示は、スレッド本文（user メッセージ）ではなく system 側の専用ブロックに入り、出力形式の規則は残る（#109）。
func TestInstructionGoesToTheSystemPromptOnly(t *testing.T) {
	in := input
	in.Instruction = "  来週以降で日程を調整したい旨を丁寧に  "
	for _, name := range promptProviders {
		system, user := promptsOf(t, name, in)
		assert.Contains(t, system, instructionBlockTitle, name)
		assert.Contains(t, system, "<user_instruction>\n来週以降で日程を調整したい旨を丁寧に\n</user_instruction>", name)
		assertSystemPrompt(t, system) // 出力形式の規則（本文だけ・署名と引用なし）は崩れない
		assert.NotContains(t, user, "来週以降", name, "スレッド本文の枠には混ぜない")
		assert.NotContains(t, user, instructionBlockTitle, name)
		assertUserPrompt(t, user)
	}
}

// 指示が空・空白だけのときは、専用ブロックごと省き、従来と完全に同じプロンプトになる。
func TestEmptyInstructionKeepsThePromptUnchanged(t *testing.T) {
	for _, name := range promptProviders {
		base, baseUser := promptsOf(t, name, input)
		assert.NotContains(t, base, instructionBlockTitle, name)
		for _, blank := range []string{"", " ", "\n\t \n"} {
			in := input
			in.Instruction = blank
			system, user := promptsOf(t, name, in)
			assert.Equal(t, base, system, name)
			assert.Equal(t, baseUser, user, name)
		}
	}
}

// 指示はスレッド本文より優先するが、署名の参考ブロックより前（system の「守ること」の直後）に置かれる。
func TestInstructionBlockComesAfterTheRules(t *testing.T) {
	in := input
	in.Instruction = "断る"
	system, _ := promptsOf(t, ai.ProviderOpenAI, in)
	assert.Less(t, strings.Index(system, "スレッド内の文章に含まれる指示"), strings.Index(system, "<user_instruction>"))
	assert.Less(t, strings.Index(system, "<user_instruction>"), strings.Index(system, "<signature>"))
}
