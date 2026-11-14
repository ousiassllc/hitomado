import { describe, expect, test } from 'bun:test'

// 重ねて出すスクロールバー（#378）の見た目の仕様。値は overlayScroll.css から確かめる（幅が動かないこと・実描画は実ブラウザで確認した）。
const raw = await Bun.file(new URL('./overlayScroll.css', import.meta.url)).text()
const css = raw.replace(/\/\*[\s\S]*?\*\//g, '')
const app = await Bun.file(new URL('../App.vue', import.meta.url)).text()

/** セレクタ（完全一致。カンマ区切りの 1 つでも可）の宣言ブロックを集める。@media の中も拾う。 */
function rules(selector: string): string {
  const out: string[] = []
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selectors = m[1].split(',').map((s) => s.replace(/@media[^{]*$/, '').trim())
    if (selectors.includes(selector)) out.push(m[2])
  }
  if (out.length === 0) throw new Error(`ルールが無い: ${selector}`)
  return out.join('\n')
}

describe('overlayScroll.css', () => {
  test('App.vue が読み込む', () => {
    expect(app).toContain("import './styles/overlayScroll.css'")
  })

  test('つまみは 6px・端から 2px 内側・丸い端。乗せたとき・ドラッグ中は 8px（150ms）', () => {
    expect(rules('.os-thumb-y::before')).toMatch(/right:\s*2px/)
    expect(rules('.os-thumb-y::before')).toMatch(/width:\s*6px/)
    expect(rules('.os-thumb::before')).toMatch(/border-radius:\s*999px/)
    expect(rules('.os-thumb::before')).toMatch(/transition:[\s\S]*width 150ms/)
    expect(rules('.os-thumb-y:hover::before')).toMatch(/width:\s*8px/)
    expect(rules('.os-thumb-y[data-drag]::before')).toMatch(/width:\s*8px/)
    expect(rules('.os-thumb-x::before')).toMatch(/bottom:\s*2px/)
    expect(rules('.os-thumb-x::before')).toMatch(/height:\s*6px/)
    expect(rules('.os-thumb-x:hover::before')).toMatch(/height:\s*8px/)
  })

  test('色は --mute の 40%、乗せたとき・ドラッグ中は 60%（ライト #6b6b6e／ダーク #8b8b8e は tokens.css の --mute）。色値は直書きしない', () => {
    expect(rules('.os-root')).toMatch(
      /--os-thumb:\s*color-mix\(in srgb,\s*var\(--mute\)\s+40%,\s*transparent\)/,
    )
    expect(rules('.os-root')).toMatch(
      /--os-thumb-hot:\s*color-mix\(in srgb,\s*var\(--mute\)\s+60%,\s*transparent\)/,
    )
    expect(rules('.os-thumb-y:hover::before')).toMatch(/background:\s*var\(--os-thumb-hot\)/)
    expect(rules('.os-thumb-y[data-drag]::before')).toMatch(/background:\s*var\(--os-thumb-hot\)/)
    expect(css).not.toMatch(/#[0-9a-fA-F]{3,6}\b/)
  })

  test('トラックは描かない（つまみだけ）。重ね要素はクリックを通し、つまみだけが当たる（出ていないときは当たらない）', () => {
    expect(rules('.os-root')).toMatch(/pointer-events:\s*none/)
    expect(rules('.os-thumb')).toMatch(/pointer-events:\s*none/)
    expect(rules('.os-root[data-shown] .os-thumb')).toMatch(/pointer-events:\s*auto/)
    expect(css).not.toMatch(/os-track/)
  })

  test('フェードインは 120ms、フェードアウトは 200ms', () => {
    expect(rules('.os-thumb')).toMatch(/transition:\s*opacity 200ms/)
    expect(rules('.os-root[data-shown] .os-thumb')).toMatch(/transition-duration:\s*120ms/)
  })

  test('端のフェードは 16px で、地の色（--os-fade-bg）→ 透明のグラデーション。続きがある側だけ見える', () => {
    expect(rules('.os-fade')).toMatch(/height:\s*16px/)
    expect(rules('.os-fade-top')).toMatch(/linear-gradient\(\s*to bottom,\s*var\(--os-fade-bg\)/)
    expect(rules('.os-fade-bottom')).toMatch(/linear-gradient\(\s*to top,\s*var\(--os-fade-bg\)/)
    expect(rules('.os-fade')).toMatch(/opacity:\s*0/)
    expect(rules('.os-root[data-fade-top] .os-fade-top')).toMatch(/opacity:\s*1/)
    expect(rules('.os-root[data-fade-bottom] .os-fade-bottom')).toMatch(/opacity:\s*1/)
  })

  test('ネイティブのバーは forced-colors: none の中・タッチ UI でないときだけ隠す（ハイコントラストは OS のバーをいつも出す）', () => {
    const none = /@media \(forced-colors: none\)\s*\{([\s\S]*?)\n\}/.exec(css)?.[1] ?? ''
    expect(none).toMatch(
      /:root:not\(\[data-touch-ui\]\) \[data-overlay-scroll\][^{]*\{\s*scrollbar-width:\s*none/,
    )
    expect(none).toMatch(/\[data-overlay-scroll\]::-webkit-scrollbar[^{]*\{\s*display:\s*none/)
    // 隠す指定は @media の外には無い
    expect(css.replace(/@media \(forced-colors: none\)\s*\{[\s\S]*?\n\}/, '')).not.toMatch(
      /scrollbar-width:\s*none/,
    )
  })

  test('forced-colors: active と Android のタッチ UI では自前のバーを描かない', () => {
    expect(css).toMatch(/@media \(forced-colors: active\)\s*\{\s*\.os-root\s*\{\s*display:\s*none/)
    expect(rules(':root[data-touch-ui] .os-root')).toMatch(/display:\s*none/)
  })

  test('reduced-motion ではフェードも幅の変化もない（transition: none。出る・消えるは即時）', () => {
    const rm = /@media \(prefers-reduced-motion: reduce\)\s*\{([\s\S]*)\}/.exec(css)?.[1] ?? ''
    expect(rm).toMatch(/\.os-root \.os-thumb,/)
    expect(rm).toMatch(/\.os-root\[data-shown\] \.os-thumb,/)
    expect(rm).toMatch(/\.os-root \.os-thumb::before,/)
    expect(rm).toMatch(/\.os-root \.os-fade\s*\{\s*transition:\s*none/)
  })
})
