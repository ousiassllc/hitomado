// Ousiass デザインガイド準拠の回帰テスト（#210）。色（C4・C6）の指定を各部品の <style> で確かめる。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'

/** コンポーネントの <style> から、セレクタ（`.x {` まで含めて指定）の宣言ブロックを返す。 */
function rule(file: string, selector: string): string {
  const style = parse(
    readFileSync(new URL(`../../src/components/${file}`, import.meta.url), 'utf8'),
  ).descriptor.styles[0].content
  const at = style.indexOf(selector)
  expect(at).toBeGreaterThanOrEqual(0)
  return style.slice(at).match(/\{([^}]*)\}/)?.[1] ?? ''
}

describe('状態ピルと同じ色（C4・#210）', () => {
  test('無効な宛先チップは文字 --danger・枠 --danger-border・地 --danger-soft', () => {
    const body = rule('compose/RecipientField.vue', '.chip-invalid {')
    expect(body).toContain('border-color: var(--danger-border)')
    expect(body).toContain('background: var(--danger-soft)')
    expect(body).toContain('color: var(--danger)')
  })
})

describe('長文・複数文は --mute にしない（C6・#210）', () => {
  test('本文エディタの署名・引用は --ink', () => {
    const sig = rule('compose/RichTextEditor.vue', '[data-hitomado-signature])')
    const quote = rule('compose/RichTextEditor.vue', '[data-hitomado-quote])')
    expect(sig).toContain('color: var(--ink)')
    expect(quote).toContain('color: var(--ink)')
  })

  test('AI モデル欄の案内（複数文）は --ink', () => {
    expect(rule('settings/ai/AIModelField.vue', '.note {')).toContain('color: var(--ink)')
  })
})
