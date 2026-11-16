import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'

// 390px 幅（640px 以下）の見せ方（#376）。happy-dom は幅から CSS を評価しないので、メディアクエリの中の宣言を読んで確かめる。
function narrow(file: string): string {
  const style =
    parse(readFileSync(new URL(file, import.meta.url), 'utf8')).descriptor.styles[0]?.content ?? ''
  const at = style.indexOf('@media (max-width: 640px)')
  expect(at).toBeGreaterThanOrEqual(0)
  return style.slice(at)
}

describe('390 幅: 行をカードにして、ラベルを上に置く（#376）', () => {
  test('機能ごとの行: 縦に積み、ラベル→選択欄の順。選択欄は横いっぱい', () => {
    const css = narrow('./AIFeatureRow.vue')
    expect(css).toMatch(/\.frow\s*\{[^}]*flex-direction: column/)
    expect(css).toMatch(/\.pick\s*\{[^}]*max-width: none/)
  })

  test('機能ごとの一覧・つなぎ先の一覧: 外枠を外し、行それぞれが枠つきのカードになる', () => {
    for (const f of ['./AIUseTab.vue', '../providers/AIConnectTab.vue']) {
      const css = narrow(f)
      expect(css).toMatch(/\.box\s*\{[^}]*border: none/)
      expect(css).toMatch(/border: 1px solid var\(--border\)/)
    }
  })

  test('つなぎ先の行: 操作は名前の下の行に回る', () => {
    expect(narrow('../providers/AIProviderRow.vue')).toMatch(/\.actions\s*\{[^}]*flex-basis: 100%/)
  })

  test('エフォート: ラベルを選択欄の上に置く', () => {
    expect(narrow('../AIEffortField.vue')).toMatch(/\.field\s*\{[^}]*flex-direction: column/)
  })
})
