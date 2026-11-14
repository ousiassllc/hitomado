import { describe, expect, test } from 'bun:test'
import { parse } from '@vue/compiler-sfc'

// reka の SelectViewport が注入する scrollbar-width: none と ::-webkit-scrollbar { display: none } は、通常は使い（重ねて出すバーを
// v-overlay-scroll が描く。#378）、OS のバーを出す場所（ハイコントラスト・Android のタッチ UI）だけ打ち消す（ガイド §10・B6）。
const path = new URL('./BaseSelect.vue', import.meta.url).pathname
const source = await Bun.file(path).text()
const { descriptor } = parse(source, { filename: path })
const css = descriptor.styles
  .map((s) => s.content)
  .join('\n')
  .replace(/\/\*[\s\S]*?\*\//g, '')

const viewport = '\\.sel-viewport\\[data-reka-select-viewport\\]'

describe('BaseSelect のドロップダウン（.sel-viewport）のスクロールバー', () => {
  test('SelectViewport に重ねて出すスクロールバーを取り付ける', () => {
    expect(source).toContain(':ref="viewportRef"')
    expect(source).toMatch(/const viewportRef = overlayScrollRef\(\)/)
  })

  test('forced-colors: active では reka の隠す指定を、詳細度の高いセレクタで打ち消す', () => {
    expect(css).toMatch(
      new RegExp(
        `@media \\(forced-colors: active\\)\\s*\\{\\s*:global\\(${viewport}\\)\\s*\\{[^}]*scrollbar-width:\\s*auto;`,
      ),
    )
    expect(css).toMatch(
      new RegExp(`:global\\(${viewport}::-webkit-scrollbar\\)\\s*\\{[^}]*display:\\s*block;`),
    )
  })

  test('Android のタッチ UI（html[data-touch-ui]）では OS の重ねて出るバーを使う', () => {
    expect(css).toMatch(
      new RegExp(
        `:global\\(:root\\[data-touch-ui\\]\\s+${viewport}\\)\\s*\\{[^}]*scrollbar-width:\\s*auto;`,
      ),
    )
    expect(css).toMatch(
      new RegExp(
        `:global\\(:root\\[data-touch-ui\\]\\s+${viewport}::-webkit-scrollbar\\)\\s*\\{[^}]*display:\\s*block;`,
      ),
    )
  })

  test('通常は自分で scrollbar-width: none や display: none を指定しない（隠すのは reka と overlayScroll.css）', () => {
    expect(css).not.toMatch(/scrollbar-width:\s*none/)
    expect(css).not.toMatch(/display:\s*none/)
  })
})
