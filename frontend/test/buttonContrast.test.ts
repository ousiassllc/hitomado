import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { mount } from '@vue/test-utils'
import SenderBadge from '../src/components/account/SenderBadge.vue'
import BaseButton from '../src/components/ui/BaseButton.vue'
import { contrastRatio } from '../src/lib/contrast'
import { rule } from './design/styleRule'

const css = readFileSync(new URL('../src/styles/tokens.css', import.meta.url), 'utf8')
const darkStart = css.indexOf('@media (prefers-color-scheme: dark)')
const darkEnd = css.indexOf('@media (prefers-reduced-motion')
if (darkStart < 0 || darkEnd <= darkStart)
  throw new Error('tokens.css のダーク区間を見つけられない')

function vars(block: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of block.matchAll(/(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{3,6})\s*;/g)) out[m[1]] = m[2]
  return out
}

const light = vars(css.slice(0, darkStart))
const dark = { ...light, ...vars(css.slice(darkStart, darkEnd)) }

describe('primary / danger ボタンの文字色コントラスト（WCAG AA 4.5:1）', () => {
  const themes: [string, Record<string, string>][] = [
    ['ライト', light],
    ['ダーク', dark],
  ]
  const variants = [
    ['primary', '--accent', '--bone'],
    ['danger', '--danger', '--bone'],
  ] as const
  for (const [theme, v] of themes) {
    for (const [name, bg, fg] of variants) {
      test(`${theme}: ${name} は 4.5:1 以上`, () => {
        expect(contrastRatio(v[bg], v[fg])).toBeGreaterThanOrEqual(4.5)
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
    // happy-dom は宣言がなければ opacity を空文字で返す。薄める宣言（0.4）が当たっていないことを確かめる（disabled 側のテストが宣言の解釈を担保する）
    expect(['', '1']).toContain(opacityOf({ variant: 'primary', loading: true }))
  })

  test('disabled の primary は薄くする（押せないことを示す）', () => {
    expect(opacityOf({ variant: 'primary', disabled: true })).toBe('0.4')
  })
})

describe('SenderBadge', () => {
  const src = readFileSync(new URL('../src/components/account/SenderBadge.vue', import.meta.url), 'utf8')

  test('アカウント色は名前の薄いチップだけに使い、カードの地・枠・文字には使わない（#347）', () => {
    const w = mount(SenderBadge, {
      props: { from: { name: '仕事用', email: 'work@example.com' }, color: 'purple' },
    })
    // カード本体（ルート）には識別色を渡さない
    expect(w.attributes('style')).toBeUndefined()
    const chip = w.get('.sender-chip')
    expect(chip.classes()).toContain('tinted')
    expect(chip.attributes('style')).toContain('--chip-tint: var(--account-purple)')
    expect(chip.text()).toBe('仕事用')
    // アドレスはチップの外（隣か下）の --ink の文字
    expect(chip.text()).not.toContain('work@example.com')
    expect(w.get('.sender-email').text()).toBe('work@example.com')
  })

  test('名前が無いときは、アドレスをチップに入れる（識別は残し、アドレスは 1 回だけ出す）', () => {
    const w = mount(SenderBadge, { props: { from: { email: 'work@example.com' }, color: 'teal' } })
    expect(w.get('.sender-chip').text()).toBe('work@example.com')
    expect(w.find('.sender-email').exists()).toBe(false)
  })

  test('カードの地はダイアログと同じ --surface、枠は --border。識別色の混色（color-mix）は持たない', () => {
    const card = rule('account/SenderBadge.vue', '.sender-badge {')
    expect(card).toContain('background: var(--surface)')
    expect(card).toContain('border: 1px solid var(--border)')
    expect(src).not.toContain('color-mix')
    expect(src).not.toContain('--chip-tint')
  })
})
