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
