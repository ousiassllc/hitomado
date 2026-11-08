import { describe, expect, test } from 'bun:test'
import { parse } from '@vue/compiler-sfc'

// reka の SelectViewport が注入する scrollbar-width: none と ::-webkit-scrollbar { display: none } を打ち消し、
// ドロップダウンのバーを常時非表示にしない（ガイド §10・B6）。
const path = new URL('./BaseSelect.vue', import.meta.url).pathname
const { descriptor } = parse(await Bun.file(path).text(), { filename: path })
const css = descriptor.styles
  .map((s) => s.content)
  .join('\n')
  .replace(/\/\*[\s\S]*?\*\//g, '')

const viewport = ':global\\(\\.sel-viewport\\[data-reka-select-viewport\\]'

describe('BaseSelect のドロップダウン（.sel-viewport）のスクロールバー', () => {
  test('reka の [data-reka-select-viewport] より詳細度の高いセレクタで scrollbar-width を none 以外に戻す', () => {
    expect(css).toMatch(
      new RegExp(
        `${viewport}\\)\\s*\\{[^}]*scrollbar-width:\\s*thin;[^}]*scrollbar-color:\\s*var\\(--mute\\)\\s+transparent;`,
      ),
    )
  })

  test('::-webkit-scrollbar 対応エンジンでは標準の 2 プロパティを auto に戻し、擬似要素の display: none を打ち消す', () => {
    expect(css).toMatch(/@supports\s+selector\(::-webkit-scrollbar\)\s*\{/)
    expect(css).toMatch(
      new RegExp(
        `${viewport}\\)\\s*\\{[^}]*scrollbar-width:\\s*auto;[^}]*scrollbar-color:\\s*auto;`,
      ),
    )
    expect(css).toMatch(
      new RegExp(`${viewport}::-webkit-scrollbar\\)\\s*\\{[^}]*display:\\s*block;`),
    )
  })

  test('scrollbar-width: none と display: none では隠さない', () => {
    expect(css).not.toMatch(/scrollbar-width:\s*none/)
    expect(css).not.toMatch(/display:\s*none/)
  })
})
