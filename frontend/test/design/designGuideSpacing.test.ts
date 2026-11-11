// Ousiass デザインガイド準拠の回帰テスト（#212）。形・余白・線（S2・S4）の指定を各部品の <style> と tokens.css で確かめる。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { rule } from './styleRule'

const tokens = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8')

describe('間隔レシピ（S2・#212）', () => {
  test('--gap-menu（メニュー・ナビ項目の項目間）は 8px', () => {
    expect(tokens).toMatch(/--gap-menu:\s*var\(--s-2\);/)
  })

  test('アカウント一覧（サイドナビ）の項目間は --gap-menu', () => {
    expect(rule('account/AccountList.vue', '.list {')).toContain('gap: var(--gap-menu)')
  })

  test('スレッドの AI パネル（小さいカード）の padding は --pad-card-sm', () => {
    expect(rule('thread/ai/ThreadAiPanel.vue', '.ai-panel {')).toContain(
      'padding: var(--pad-card-sm)',
    )
  })
})

describe('線（S4・#212）', () => {
  test('本文エディタの引用の左線は 1px solid --mute（太い枠を使わない）', () => {
    expect(rule('compose/RichTextEditor.vue', '[data-hitomado-quote])')).toContain(
      'border-left: 1px solid var(--mute)',
    )
  })
})

describe('入れ子パネルの padding（S2・#223）', () => {
  test('ChatGPT アカウント一覧の枠は --pad-card-sm', () => {
    expect(rule('settings/ai/AIChatGPTLogin.vue', '.accounts {')).toContain(
      'padding: var(--pad-card-sm)',
    )
  })

  test('データの送信先のパネルも --pad-card-sm', () => {
    expect(rule('settings/ai/AIDestinationBox.vue', '.dest-panel {')).toContain(
      'padding: var(--pad-card-sm)',
    )
  })
})

describe('部品間の間隔（S2・#228）', () => {
  test('ラベル→入力は --gap-label、入力→補足（フッタ）は --gap-help', () => {
    for (const [file, root, foot] of [
      ['compose/ai/AiDraftInstruction.vue', '.instruction {', '.instruction-foot {'],
      ['thread/ai/ThreadAiQuestionForm.vue', '.question {', '.question-foot {'],
    ]) {
      expect(rule(file, root)).toContain('gap: var(--gap-label)')
      expect(rule(file, foot)).toContain('margin-top: calc(var(--gap-help) - var(--gap-label))')
    }
  })

  test('枠付きカードの並び（OAuth クライアント一覧）は --s-3（AccountsSettings と同じ）', () => {
    expect(rule('settings/oauth/OAuthClientsSettings.vue', '.list {')).toContain('gap: var(--s-3)')
  })

  test('ボタン列・インラインの間隔は --gap-inline（ReplyBar・Cc/Bcc 切り替え）', () => {
    expect(rule('thread/ReplyBar.vue', '.reply-bar {')).toContain('gap: var(--gap-inline)')
    expect(rule('compose/ComposeFields.vue', '.toggles {')).toContain('gap: var(--gap-inline)')
  })

  test('送信確認の警告パネルの積みは --s-2（AlertStack と同じ）', () => {
    expect(rule('compose/SendConfirmDialog.vue', '.warnings {')).toContain('gap: var(--s-2)')
  })
})

describe('グループ間・まとまり間の間隔（S2・#237）', () => {
  test('カード内のまとまり同士は --gap-card-section（24）。AI パネルがそれを使う', () => {
    expect(tokens).toMatch(/--gap-card-section:\s*var\(--s-6\);/)
    expect(rule('thread/ai/ThreadAiPanel.vue', '.ai-panel {')).toContain(
      'gap: var(--gap-card-section)',
    )
  })

  test('設定画面のグループ間は --gap-field（16）以上、組の中は --s-2', () => {
    expect(rule('settings/McpSettings.vue', '.mcp,')).toContain('gap: var(--gap-field)')
    expect(rule('settings/McpSettings.vue', '.group {')).toContain('gap: var(--s-2)')
    for (const [file, selector] of [
      ['settings/LogSettings.vue', '.logs {'],
      ['settings/ai/AIChatGPTLogin.vue', '.chatgpt {'],
      ['settings/ai/AIConsentSection.vue', '.consent {'],
    ]) {
      expect(rule(file, selector)).toContain('gap: var(--gap-field)')
    }
  })

  test('スイッチは flex の行で縮まない（36×20 を保つ）', () => {
    expect(rule('ui/BaseSwitch.vue', '.sw {')).toContain('flex: none')
  })
})

describe('段落間・見出し→本文・ラベル→内容の間隔（S2・#242）', () => {
  test('--gap-para（本文の段落間）は 16px。本文エディタの段落・箇条書きがそれを使う', () => {
    expect(tokens).toMatch(/--gap-para:\s*var\(--s-4\);/)
    expect(rule('compose/RichTextEditor.vue', '.editor-content p)')).toContain(
      'margin: 0 0 var(--gap-para)',
    )
    expect(rule('compose/RichTextEditor.vue', '.editor-content ul)')).toContain(
      'margin: 0 0 var(--gap-para)',
    )
  })

  test('見出し→本文は 8px（--s-2）: 質問/回答（.turn）と利用枠超過の注意（.limit-main）', () => {
    expect(rule('thread/ai/ThreadAiPanel.vue', '.turn {')).toContain('gap: var(--s-2)')
    expect(rule('compose/ai/UsageLimitNotice.vue', '.limit-main {')).toContain('gap: var(--s-2)')
  })

  test('送信確認の送信元のラベル→内容は --gap-label', () => {
    expect(rule('compose/SendConfirmDialog.vue', '.confirm-from {')).toContain(
      'gap: var(--gap-label)',
    )
  })
})

describe('入れ子パネルの地・末尾の余白・組どうしの間隔（S1・S2・#243）', () => {
  test('ChatGPT アカウント一覧の枠（入れ子パネル）は --bone の地', () => {
    expect(rule('settings/ai/AIChatGPTLogin.vue', '.accounts {')).toContain(
      'background: var(--bone)',
    )
  })

  test('本文エディタの末尾の段落・箇条書きは、署名・引用の中の最後の p も含めて下の余白を持たない（#244）', () => {
    // 直下の子（> :last-child）だけでは、署名・引用（div / blockquote）の中の p の 16 が外へ突き抜けて末尾に残る
    expect(rule('compose/RichTextEditor.vue', '.editor-content :is(p, ul):last-child)')).toContain(
      'margin-bottom: 0',
    )
  })

  test('署名・引用の上の間隔は段落間（--gap-para）と同じ（#244）', () => {
    expect(rule('compose/RichTextEditor.vue', '[data-hitomado-signature])')).toContain(
      'margin-top: var(--gap-para)',
    )
    expect(rule('compose/RichTextEditor.vue', '[data-hitomado-quote])')).toContain(
      'margin: var(--gap-para) 0 0',
    )
  })

  test('会話の質問/回答の組どうしは --gap-para。組の中の見出し→本文（.turn）は --s-2 のまま', () => {
    expect(rule('thread/ai/ThreadAiPanel.vue', '.conversation {')).toContain('gap: var(--gap-para)')
    expect(rule('thread/ai/ThreadAiPanel.vue', '.turn {')).toContain('gap: var(--s-2)')
  })
})
