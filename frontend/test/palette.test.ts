import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { contrastRatio, parseHex, relativeLuminance } from '../src/lib/contrast'
import {
  ACCOUNT_COLOR_KEYS,
  accountColorVar,
  accountTextVar,
  isAccountColorKey,
} from '../src/lib/palette'

const css = readFileSync(new URL('../src/styles/tokens.css', import.meta.url), 'utf8')
const darkStart = css.indexOf('@media (prefers-color-scheme: dark)')
const darkEnd = css.indexOf('@media (prefers-reduced-motion')

function vars(block: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of block.matchAll(/(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{3,6})\s*;/g)) out[m[1]] = m[2]
  return out
}

const light = vars(css.slice(0, darkStart))
const dark = { ...light, ...vars(css.slice(darkStart, darkEnd)) }

// 帯・チップが載り得る背景。ページ（--bone）とカード（--surface）では 4.5:1、ホバー/選択（--accent-soft）では
// 非テキストの UI（帯）の基準 3:1 を満たす。
const BACKGROUNDS = ['--bone', '--surface']
const HOVER_BACKGROUND = '--accent-soft'

describe('contrast', () => {
  test('白と黒は 21:1', () => {
    expect(contrastRatio('#ffffff', '#000000')).toBeCloseTo(21, 5)
    expect(contrastRatio('#000', '#fff')).toBeCloseTo(21, 5)
  })

  test('同色は 1:1', () => {
    expect(contrastRatio('#336699', '#336699')).toBeCloseTo(1, 5)
  })

  test('相対輝度（sRGB の閾値付近）', () => {
    expect(relativeLuminance(parseHex('#ffffff'))).toBeCloseTo(1, 5)
    expect(relativeLuminance(parseHex('#000000'))).toBe(0)
    expect(relativeLuminance(parseHex('#808080'))).toBeCloseTo(0.2159, 3)
  })

  test('不正な色は例外', () => {
    expect(() => parseHex('red')).toThrow()
  })
})

describe('ガイドの色トークン', () => {
  test('文字（--ink）は背景（--bone・--surface）で 4.5:1 以上（ライト・ダーク）', () => {
    for (const v of [light, dark]) {
      for (const name of ['--bone', '--surface']) {
        expect(contrastRatio(v['--ink'], v[name])).toBeGreaterThanOrEqual(4.5)
      }
    }
  })

  test('補助テキスト（--mute）は通常の文字色より薄い（区別できる）', () => {
    for (const v of [light, dark]) {
      expect(contrastRatio(v['--mute'], v['--bone'])).toBeLessThan(
        contrastRatio(v['--ink'], v['--bone']),
      )
    }
  })

  test('状態色（--success・--warning・--danger・--info）は背景（--bone・--surface）で 4.5:1 以上', () => {
    for (const v of [light, dark]) {
      for (const state of ['--success', '--warning', '--danger', '--info']) {
        for (const name of ['--bone', '--surface']) {
          expect(contrastRatio(v[state], v[name])).toBeGreaterThanOrEqual(4.5)
        }
      }
    }
  })
})

describe('palette', () => {
  test('10 色・Go 側と同じ順序', () => {
    expect([...ACCOUNT_COLOR_KEYS]).toEqual([
      'red',
      'orange',
      'amber',
      'green',
      'teal',
      'cyan',
      'blue',
      'indigo',
      'purple',
      'pink',
    ])
    expect(isAccountColorKey('teal')).toBe(true)
    expect(isAccountColorKey('black')).toBe(false)
    expect(accountColorVar('red')).toBe('var(--account-red)')
    expect(accountTextVar('red')).toBe('var(--account-red-text)')
  })

  const themes: [string, Record<string, string>][] = [
    ['ライト', light],
    ['ダーク', dark],
  ]
  for (const [theme, v] of themes) {
    for (const key of ACCOUNT_COLOR_KEYS) {
      test(`${theme}: ${key} は帯はページ・カードの背景で 4.5:1 以上（ホバーでは 3:1 以上）、チップ文字は 4.5:1 以上`, () => {
        const bg = v[`--account-${key}`]
        const fg = v[`--account-${key}-text`]
        expect(bg).toBeDefined()
        expect(fg).toBeDefined()
        for (const name of BACKGROUNDS) {
          expect(contrastRatio(bg, v[name])).toBeGreaterThanOrEqual(4.5)
        }
        expect(contrastRatio(bg, v[HOVER_BACKGROUND])).toBeGreaterThanOrEqual(3)
        expect(contrastRatio(bg, fg)).toBeGreaterThanOrEqual(4.5)
      })
    }
  }
})
