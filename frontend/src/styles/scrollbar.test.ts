import { describe, expect, test } from 'bun:test'

// Ousiass デザインガイド（§10）のトークンで描く、重ねて出すバーが付かない領域の既定のスクロールバー（#208・#289）。重ねて出す本体は overlayScroll.css（overlayScroll.test.ts）。値は scrollbar.css から確かめる。
const raw = await Bun.file(new URL('./scrollbar.css', import.meta.url)).text()
const css = raw.replace(/\/\*[\s\S]*?\*\//g, '')
const app = await Bun.file(new URL('../App.vue', import.meta.url)).text()

/** `opener {` から対応する `}` までの中身を返す（入れ子の波括弧を数える）。 */
function block(source: string, opener: RegExp): string {
  const m = opener.exec(source)
  if (!m) throw new Error(`ブロックが無い: ${opener}`)
  let depth = 1
  for (let i = m.index + m[0].length; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(m.index + m[0].length, i)
  }
  throw new Error(`閉じていない: ${opener}`)
}

const TOUCH_GUARD = ':root:not([data-touch-ui]) '
const forcedColorsNone = block(css, /@media\s*\(forced-colors:\s*none\)\s*\{/)
const supportsWithGuard = block(
  forcedColorsNone,
  /@supports\s+selector\(::-webkit-scrollbar\)\s*\{/,
)
const outsideSupportsWithGuard = forcedColorsNone.replace(supportsWithGuard, '')
// 詳細は :root:not([data-touch-ui]) を外して見る（外す前の形は「タッチ UI に指定しない」のテストが確かめる）
const supports = supportsWithGuard.replaceAll(TOUCH_GUARD, '')
const outsideSupports = outsideSupportsWithGuard.replaceAll(TOUCH_GUARD, '')

describe('scrollbar.css', () => {
  test('カスタムはすべて @media (forced-colors: none) の中に置く（ハイコントラストでは OS の配色に任せる）', () => {
    expect(
      css
        .replace(forcedColorsNone, '')
        .replace(/@media\s*\(forced-colors:\s*none\)\s*\{\s*\}/, '')
        .trim(),
    ).toBe('')
  })

  test('* に細いスクロールバーを指定する（scrollbar-width は継承されないので :root だけでは非ルートの領域に効かない）。Android のタッチ UI は除く', () => {
    expect(outsideSupports).toMatch(/\*\s*\{[^}]*scrollbar-width:\s*thin;/)
  })

  test('Android のタッチ UI（html[data-touch-ui]）には何も指定しない（OS の重ねて出るスクロールバーを使う。#378）', () => {
    const selectors = [...forcedColorsNone.matchAll(/([^{}@]+)\{/g)]
      .flatMap((m) => m[1].split(','))
      .map((sel) => sel.trim())
      .filter((sel) => sel && !sel.startsWith('supports '))
    expect(selectors.length).toBeGreaterThan(0)
    for (const sel of selectors) expect(sel.startsWith(TOUCH_GUARD.trim())).toBe(true)
  })

  test('つまみ・溝の色はトークンで指定し、色値は直書きしない（ライト／ダークはトークン側が切り替える）', () => {
    expect(outsideSupports).toMatch(/\*\s*\{[^}]*scrollbar-color:\s*var\(--mute\)\s+transparent;/)
    expect(css).not.toMatch(/#[0-9a-fA-F]{3,6}\b/)
  })

  test('::-webkit-scrollbar 対応エンジンでは標準の 2 プロパティを auto に戻す（Chromium 121+ は auto 以外だと擬似要素を無視する）', () => {
    expect(supports).toMatch(/\*\s*\{[^}]*scrollbar-width:\s*auto;[^}]*scrollbar-color:\s*auto;/)
  })

  test('::-webkit-scrollbar は幅・高さ 8px、矢印なし、透明な track・corner', () => {
    expect(supports).toMatch(/::-webkit-scrollbar\s*\{[^}]*width:\s*8px;[^}]*height:\s*8px;/)
    expect(supports).toMatch(
      /::-webkit-scrollbar-track,\s*::-webkit-scrollbar-corner\s*\{[^}]*background:\s*transparent;/,
    )
    expect(supports).toMatch(/::-webkit-scrollbar-button\s*\{[^}]*display:\s*none;/)
  })

  test('つまみは --mute・角丸 999px、hover は --ink 35%・active は 65% を混ぜる', () => {
    expect(supports).toMatch(
      /::-webkit-scrollbar-thumb\s*\{[^}]*background:\s*var\(--mute\);[^}]*border-radius:\s*999px;/,
    )
    expect(supports).toMatch(
      /::-webkit-scrollbar-thumb:hover\s*\{[^}]*background:\s*color-mix\(in srgb,\s*var\(--ink\)\s+35%,\s*var\(--mute\)\);/,
    )
    expect(supports).toMatch(
      /::-webkit-scrollbar-thumb:active\s*\{[^}]*background:\s*color-mix\(in srgb,\s*var\(--ink\)\s+65%,\s*var\(--mute\)\);/,
    )
  })

  test('App.vue が読み込む', () => {
    expect(app).toContain("import './styles/scrollbar.css'")
  })
})
