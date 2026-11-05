import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { mount } from '@vue/test-utils'
import SenderBadge from '../src/components/account/SenderBadge.vue'
import BaseButton from '../src/components/ui/BaseButton.vue'
import { contrastRatio } from '../src/lib/contrast'

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

/** `filter: brightness(k)` を #rrggbb にかけた色（hover で使う）。 */
function brighten(hex: string, k: number): string {
  const n = Number.parseInt(hex.slice(1), 16)
  const ch = [n >> 16, (n >> 8) & 255, n & 255].map((v) => Math.min(255, Math.round(v * k)))
  return `#${ch.map((v) => v.toString(16).padStart(2, '0')).join('')}`
}

describe('primary / danger ボタンの文字色コントラスト（WCAG AA 4.5:1）', () => {
  const themes: [string, Record<string, string>][] = [
    ['ライト', light],
    ['ダーク', dark],
  ]
  const variants = [
    ['primary', '--color-accent', '--color-accent-text'],
    ['danger', '--color-danger', '--color-danger-text'],
  ] as const
  for (const [theme, v] of themes) {
    for (const [name, bg, fg] of variants) {
      test(`${theme}: ${name} は通常・hover（brightness 1.1）とも 4.5:1 以上`, () => {
        expect(contrastRatio(v[bg], v[fg])).toBeGreaterThanOrEqual(4.5)
        expect(contrastRatio(brighten(v[bg], 1.1), v[fg])).toBeGreaterThanOrEqual(4.5)
      })
    }
  }
})

// 送信確認ダイアログの「送信中…」で、primary が opacity で薄まり白文字が読めなくなっていた（ライト 2.5:1）。
// happy-dom でも宣言どおりの opacity は解決されるので、SFC の style を実際に当てて確かめる。
describe('BaseButton の状態別の見た目', () => {
  const styleBlock = parse(
    readFileSync(new URL('../src/components/ui/BaseButton.vue', import.meta.url), 'utf8'),
  ).descriptor.styles[0].content
  const sheet = document.createElement('style')
  sheet.textContent = styleBlock

  function opacityOf(props: Record<string, unknown>): string {
    document.head.appendChild(sheet)
    const w = mount(BaseButton, { props, slots: { default: '送信' }, attachTo: document.body })
    const op = getComputedStyle(w.element).opacity
    w.unmount()
    sheet.remove()
    return op
  }

  test('loading（送信中）の primary は薄くしない', () => {
    expect(opacityOf({ variant: 'primary', loading: true })).not.toBe('0.55')
  })

  test('disabled の primary は薄くする（押せないことを示す）', () => {
    expect(opacityOf({ variant: 'primary', disabled: true })).toBe('0.55')
  })
})

describe('SenderBadge', () => {
  test('アカウント色は左の帯だけに使い、背景や文字色には使わない', () => {
    const w = mount(SenderBadge, {
      props: { from: { name: '仕事用', email: 'work@example.com' }, color: 'purple' },
    })
    const style = (w.element as HTMLElement).getAttribute('style') ?? ''
    expect(style).toContain('border-left-color: var(--account-purple)')
    expect(style).not.toContain('background')
    expect(style).not.toMatch(/(^|[;\s])color:/)
    expect(w.text()).toContain('仕事用')
    expect(w.text()).toContain('work@example.com')
  })
})
