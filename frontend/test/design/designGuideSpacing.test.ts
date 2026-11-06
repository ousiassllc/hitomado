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
