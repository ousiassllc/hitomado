import { describe, expect, test } from 'bun:test'
import { parse } from '@vue/compiler-sfc'

// フォーカスリング（外側 offset 2 + 2px）が、スクロール領域の縁で切れないことの回帰テスト（#197）。
// テストの SFC ローダーは <style> を取り込まないので、SFC の CSS を読んで確かめる。
async function styleOf(file: string): Promise<string> {
  const path = new URL(file, import.meta.url).pathname
  const { descriptor } = parse(await Bun.file(path).text(), { filename: path })
  return descriptor.styles.map((s) => s.content).join('\n')
}

describe('BaseDialog の本文', () => {
  test('縦スクロールの本文は四辺に --s-1 の余白を持ち、同じ分だけ負の margin で位置を保つ', async () => {
    const css = await styleOf('./BaseDialog.vue')
    const body = css.match(/\.dlg-body\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(body).toMatch(/overflow:\s*auto;/)
    expect(body).toMatch(/padding:\s*var\(--s-1\);/)
    expect(body).toMatch(
      /margin:\s*calc\(var\(--s-3\) - var\(--s-1\)\) calc\(var\(--s-1\) \* -1\) calc\(var\(--s-1\) \* -1\);/,
    )
  })
})

describe('SettingsDialog の横スクロールするタブ', () => {
  test('フォーカスリングは内側（offset -2px）に描く', async () => {
    const css = await styleOf('../settings/SettingsDialog.vue')
    const rule = css.match(/\.tab-list\.scroll \.tab:focus-visible\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/outline:\s*2px solid var\(--focus\);/)
    expect(rule).toMatch(/outline-offset:\s*-2px;/)
  })
})

// メニュー・セレクトの強調項目（data-highlighted）はポインタの移動でも付くので、ホバーは地だけにする（#258）。
// キーボードの強調は reka が項目へ DOM フォーカスを当てるので、:focus-visible のときだけリングを描く（#252）。
describe.each([
  ['BaseMenu', './BaseMenu.vue', '\\.menu-item'],
  ['BaseSelect', './BaseSelect.vue', '\\.sel-item'],
])('%s の強調項目', (_name, file, item) => {
  test('ホバー（data-highlighted だけ）は地のみで、枠を描かない', async () => {
    const css = await styleOf(file)
    const rule = css.match(new RegExp(`${item}\\[data-highlighted\\]\\s*\\{([^}]*)\\}`))?.[1] ?? ''
    expect(rule).toMatch(/background:\s*var\(--accent-soft\);/)
    expect(rule).not.toMatch(/outline/)
  })

  test('キーボード操作（:focus-visible）のときだけ --focus のリングを内側（offset -2px）に描く', async () => {
    const css = await styleOf(file)
    const rule =
      css.match(new RegExp(`${item}\\[data-highlighted\\]:focus-visible\\s*\\{([^}]*)\\}`))?.[1] ??
      ''
    expect(rule).toMatch(/outline:\s*2px solid var\(--focus\);/)
    expect(rule).toMatch(/outline-offset:\s*-2px;/)
  })
})
