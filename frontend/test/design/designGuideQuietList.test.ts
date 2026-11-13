// 一覧の識別色・未読・削除・余白を静かにそろえる回帰テスト（#345）。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import LabelChip from '../../src/components/inbox/LabelChip.vue'
import ListToolbar from '../../src/components/inbox/ListToolbar.vue'
import MessageCard from '../../src/components/thread/MessageCard.vue'
import { rowHeightOf } from '../../src/lib/inboxItems'
import { rule } from './styleRule'

setActivePinia(createPinia())

const tokens = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8')

describe('識別色のチップ（#345）', () => {
  test('チップの地は識別色を --surface に混ぜた薄い色、文字は --ink、枠は 1px の同系色を薄めたもの', () => {
    const chip = rule('ui/BaseChip.vue', '.chip.tinted {')
    expect(chip).toContain(
      'background: color-mix(in srgb, var(--chip-tint) var(--tint-bg), var(--surface))',
    )
    expect(chip).toContain(
      'border-color: color-mix(in srgb, var(--chip-tint) var(--tint-border), transparent)',
    )
    const base = rule('ui/BaseChip.vue', '.chip {')
    expect(base).toContain('color: var(--ink)')
    expect(base).toContain('border: 1px solid')
  })

  test('混ぜる割合は地 15〜20%（識別色は沈めて使う）', () => {
    const pct = Number(tokens.match(/--tint-bg:\s*(\d+)%;/)?.[1])
    expect(pct).toBeGreaterThanOrEqual(15)
    expect(pct).toBeLessThanOrEqual(20)
  })

  test('Gmail の生の colorBg・colorFg は塗らない（colorBg は色相の出所だけ、colorFg は使わない）', () => {
    const w = mount(LabelChip, {
      props: { label: { id: 'L1', name: '重要', colorBg: '#ff0000', colorFg: '#ffffff' } },
    })
    const style = w.attributes('style') ?? ''
    expect(style).toContain('--chip-tint: #ff0000')
    expect(style).not.toContain('#ffffff')
    expect(style).not.toMatch(/(^|;)\s*(background|color):/)
  })

  test('色の指定が無いラベルは既定の見た目のまま（tinted にならない）', () => {
    const w = mount(LabelChip, { props: { label: { id: 'L2', name: '色なし' } } })
    expect(w.classes()).not.toContain('tinted')
  })
})

describe('スレッド行（#345）', () => {
  test('行の padding は縦 --s-3・横 --pad-card-sm（--s-4）。帯は無い', () => {
    expect(rule('inbox/ThreadRow.vue', '.main {')).toContain(
      'padding: var(--s-3) var(--pad-card-sm)',
    )
    const src = readFileSync(
      new URL('../../src/components/inbox/ThreadRow.vue', import.meta.url),
      'utf8',
    )
    expect(src).not.toContain('.band')
  })

  test('行左の間隔は --gap-inline', () => {
    expect(rule('inbox/ThreadRow.vue', '.line1,')).toContain('gap: var(--gap-inline)')
  })

  test('現在行・選択・複数選択・キーボードカーソルの地は --accent-soft、行自体は --focus の囲みを持たない', () => {
    const src = readFileSync(
      new URL('../../src/components/inbox/ThreadRow.vue', import.meta.url),
      'utf8',
    )
    expect(rule('inbox/ThreadRow.vue', '.row:hover,')).toContain('background: var(--accent-soft)')
    expect(src).toContain('.row.active {')
    expect(src).not.toContain('--focus')
  })

  test('--focus の囲みは listbox にキーボードフォーカスがあるときだけ、カーソル行に出る', () => {
    expect(rule('inbox/ThreadList.vue', '.inner:focus-visible :deep(.row.active) {')).toContain(
      'outline: 2px solid var(--focus)',
    )
  })

  test('ラベルのある行だけ高い（padding 縦 12 ×2 を保ったまま、ラベルの行が収まる）', () => {
    const base = { labels: [] } as unknown as ThreadSummary
    const labeled = { labels: [{ id: 'L1', name: '重要' }] } as unknown as ThreadSummary
    expect(rowHeightOf(base)).toBe(80)
    expect(rowHeightOf(labeled)).toBe(104)
    expect(rowHeightOf({} as ThreadSummary)).toBe(80)
  })
})

describe('ツールバーの削除（#345）', () => {
  test('通常時はほかのアイコンと同じインク色（danger にしない）', () => {
    const w = mount(ListToolbar, {
      props: {
        selectedCount: 2,
        refreshing: false,
        labelEnabled: true,
        labels: [],
        labelsLoading: false,
      },
    })
    const trash = w.get('button[aria-label="ゴミ箱へ移動"]')
    expect(trash.classes()).toContain('icon-btn-ghost')
    expect(trash.classes()).not.toContain('icon-btn-danger')
  })
})

describe('メッセージカードの未読（#345）', () => {
  const message = {
    id: 'm1',
    from: { name: '山田太郎', email: 'y@example.com' },
    to: [],
    date: Date.now(),
    isUnread: true,
    bodyText: '本文',
    attachments: [],
  } as never

  test('「未読」の塗りピルは無く、点と差出人の太さで示す（読み上げ用の文字は見えない）', () => {
    const w = mount(MessageCard, { props: { message, expanded: false } })
    expect(w.find('.unread-badge').exists()).toBe(false)
    expect(w.get('.dot').attributes('aria-hidden')).toBe('true')
    // 読み上げには、開閉ボタンの名前の一部として残す（見た目には出さない）
    expect(w.get('.sr-only').text()).toBe('未読')
    expect(rule('thread/MessageCard.vue', '.is-unread .sender {')).toContain('font-weight: 600')
  })

  test('既読のカードには点が無い', () => {
    const w = mount(MessageCard, {
      props: { message: { ...(message as object), isUnread: false } as never, expanded: false },
    })
    expect(w.find('.dot').exists()).toBe(false)
  })
})
