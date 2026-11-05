import { describe, expect, test } from 'bun:test'

// docs/components/overview.md §5.3 の逸脱（タッチ UI のタップ領域 44px）と touch.css の値を揃えておく。
const css = await Bun.file(new URL('./touch.css', import.meta.url)).text()

describe('touch.css', () => {
  test('--tap-min は 44px で、押せる要素と入力欄の最小高さに使う', () => {
    expect(css).toMatch(/:root\[data-touch-ui\]\s*\{\s*--tap-min:\s*44px;/)
    expect(css).toMatch(/\.btn,[\s\S]*?\{\s*min-height:\s*var\(--tap-min\);/)
    expect(css).toMatch(/input\.text\s*\{\s*min-height:\s*var\(--tap-min\);/)
  })

  test('スイッチの当たり判定は疑似要素で上下 12px・左右 2px 広げる', () => {
    expect(css).toMatch(/\.sw::after\s*\{[^}]*inset:\s*-12px -2px;/)
  })

  test('チップの削除ボタンの当たり判定は全周 11px 広げる', () => {
    expect(css).toMatch(/\.chip-remove::after\s*\{[^}]*inset:\s*-11px;/)
  })
})
