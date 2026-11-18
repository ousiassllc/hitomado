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

// × ボタンのリングは --accent（ライト＝インク・ダーク＝ボーン色）。キーボード（:focus-visible）のときだけ出し、
// マウスのクリック（:focus だけ）では出さない（#376・#377）。
describe('BaseDialog の × ボタン', () => {
  test('フォーカスリングは :focus-visible のときだけ、--accent の 2px・offset 2px', async () => {
    const css = await styleOf('./BaseDialog.vue')
    const rule = css.match(/\.dlg-close:focus-visible\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/outline:\s*2px solid var\(--accent\);/)
    expect(rule).toMatch(/outline-offset:\s*2px;/)
    expect(rule).not.toMatch(/--focus/)
    expect(css).not.toMatch(/\.dlg-close:focus\s*[{,]/)
  })
})

// 開いた直後の初期フォーカスは、操作する要素ではないダイアログ本体（tabindex="-1"）に置くので、本体にはフォーカスの枠を描かない（#376・#377）。
describe('BaseDialog の本体', () => {
  test('フォーカスされても枠を描かない', async () => {
    const css = await styleOf('./BaseDialog.vue')
    const rule = css.match(/\.dlg:focus\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/outline:\s*none;/)
  })
})

describe('SettingsDialog の横スクロールするタブ', () => {
  test('フォーカスリングは内側（offset -2px）に描く', async () => {
    const css = await styleOf('../settings/SettingsDialog.vue')
    const rule = css.match(/\.tab-list\.scroll \.tab:focus-visible\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/outline:\s*2px solid var\(--accent\);/)
    expect(rule).toMatch(/outline-offset:\s*-2px;/)
  })
})

// セレクトの強調項目（data-highlighted）はポインタの移動でも付くので、ホバーは地だけにする（#258）。
// キーボードの強調は reka が項目へ DOM フォーカスを当てるので、:focus-visible のときだけリングを描く（#252）。
// メニュー（BaseMenu）の強調行は地（--accent-soft）だけで、リングを描かない（フォーカスはメニュー全体のもの。#283）。
describe('BaseSelect の強調項目', () => {
  test('ホバー（data-highlighted だけ）は地のみで、枠を描かない', async () => {
    const css = await styleOf('./BaseSelect.vue')
    const rule = css.match(/\.sel-item\[data-highlighted\]\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/background:\s*var\(--accent-soft\);/)
    expect(rule).not.toMatch(/outline/)
  })

  test('キーボード操作（:focus-visible）のときだけリングを外側（offset 2px）に描く', async () => {
    const css = await styleOf('./BaseSelect.vue')
    const rule = css.match(/\.sel-item\[data-highlighted\]:focus-visible\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toContain('outline: 2px solid var(--accent);')
    expect(rule).toMatch(/outline-offset:\s*2px;/)
  })

  test('項目を包むスクロール領域は、外側のリング（offset 2px + 幅 2px）が収まる --s-1（4px）の余白を持つ', async () => {
    const css = await styleOf('./BaseSelect.vue')
    const rule = css.match(/:global\(\.sel-viewport\)\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/padding:\s*var\(--s-1\);/)
  })
})

describe('BaseMenu の強調行', () => {
  test('強調（data-highlighted）は地のみで、行に outline を描かない（キーボード操作でも）', async () => {
    const css = await styleOf('./BaseMenu.vue')
    const rule = css.match(/\.menu-item\[data-highlighted\]\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/background:\s*var\(--accent-soft\);/)
    expect(css).not.toMatch(/:focus-visible/)
  })
})

// Sidebar のアカウント一覧は縦スクロール領域で、先頭の「すべて」ボタンのリングが上端で切れないこと（#302）。
describe('Sidebar のアカウント一覧', () => {
  test('スクロール領域は、先頭ボタンの外側のリングが収まる --s-1（4px）の上の余白を持つ', async () => {
    const css = await styleOf('../shell/Sidebar.vue')
    const rule = css.match(/\.scroll\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(rule).toMatch(/overflow-y:\s*auto;/)
    expect(rule).toMatch(/padding:\s*var\(--s-1\) var\(--s-2\) var\(--s-2\);/)
  })
})
