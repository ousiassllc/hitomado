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

// 帯・チップが載り得る背景。--color-bg は必須、それ以外は余裕の確認。
const BACKGROUNDS = [
  '--color-bg',
  '--color-bg-subtle',
  '--color-bg-elevated',
  '--color-selected',
  '--color-unread-bg',
]

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

describe('淡色トークン', () => {
  test('--color-text-faded は行が載り得るすべての背景で 4.5:1 以上（ライト・ダーク）', () => {
    for (const v of [light, dark]) {
      for (const name of BACKGROUNDS) {
        expect(contrastRatio(v['--color-text-faded'], v[name])).toBeGreaterThanOrEqual(4.5)
      }
    }
  })

  test('淡色は通常の文字色より薄い（区別できる）', () => {
    for (const v of [light, dark]) {
      expect(contrastRatio(v['--color-text-faded'], v['--color-bg'])).toBeLessThan(
        contrastRatio(v['--color-text'], v['--color-bg']),
      )
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
      test(`${theme}: ${key} は帯・チップ文字とも 4.5:1 以上`, () => {
        const bg = v[`--account-${key}`]
        const fg = v[`--account-${key}-text`]
        expect(bg).toBeDefined()
        expect(fg).toBeDefined()
        for (const name of BACKGROUNDS) {
          expect(contrastRatio(bg, v[name])).toBeGreaterThanOrEqual(4.5)
        }
        expect(contrastRatio(bg, fg)).toBeGreaterThanOrEqual(4.5)
      })
    }
  }
})
