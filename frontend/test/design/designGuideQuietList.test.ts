// 一覧の識別色・未読・削除・余白を静かにそろえる回帰テスト（#345）。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import LabelChip from '../../src/components/inbox/LabelChip.vue'
import ListToolbar from '../../src/components/inbox/ListToolbar.vue'
import ThreadList from '../../src/components/inbox/ThreadList.vue'
import MessageCard from '../../src/components/thread/MessageCard.vue'
import { contrastRatio, parseHex } from '../../src/lib/contrast'
import { rowHeightOf } from '../../src/lib/inboxItems'
import { ACCOUNT_COLOR_KEYS } from '../../src/lib/palette'
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
        unreadOnly: false,
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

describe('ラベルの増減で一覧の高さが追従する（#345）', () => {
  const base = {
    accountId: 'a1',
    threadId: 't0',
    subject: '件名',
    snippet: '',
    lastMessageAt: 0,
    lastFrom: { name: '山田', email: 'y@example.com' },
    messageCount: 1,
    isUnread: false,
    hasAttachment: false,
    inInbox: true,
    labels: [],
  } as unknown as ThreadSummary

  test('その場でラベルが付く・外れると、listbox の全高（.inner の height）が 24 変わる', async () => {
    const items = [base, { ...base, threadId: 't1' }]
    const w = mount(ThreadList, {
      props: {
        items,
        checkedKeys: [],
        loading: false,
        hasMore: false,
        loadingMore: false,
        emptyText: '',
      },
    })
    const total = () => (w.get('.inner').element as HTMLElement).style.height
    expect(total()).toBe('160px')
    const label = { id: 'L1', name: '重要' }
    await w.setProps({ items: [{ ...items[0], labels: [label] }, items[1]] })
    expect(total()).toBe('184px')
    await w.setProps({ items })
    expect(total()).toBe('160px')
  })
})

describe('薄い地の上の --ink のコントラストと見分け（#345）', () => {
  const darkAt = tokens.indexOf('@media (prefers-color-scheme: dark)')
  const darkEnd = tokens.indexOf('@media (prefers-reduced-motion')
  const hexes = (block: string) => {
    const out: Record<string, string> = {}
    for (const m of block.matchAll(/(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{6})\s*;/g)) out[m[1]] = m[2]
    return out
  }
  const light = hexes(tokens.slice(0, darkAt))
  const dark = { ...light, ...hexes(tokens.slice(darkAt, darkEnd)) }
  const pct = Number(tokens.match(/--tint-bg:\s*(\d+)%;/)?.[1]) / 100

  // color-mix(in srgb, tint P%, surface) と同じ、sRGB の線形補間
  function mix(tint: string, surface: string): string {
    const t = parseHex(tint)
    const s = parseHex(surface)
    const c = (['r', 'g', 'b'] as const).map((k) => Math.round(t[k] * pct + s[k] * (1 - pct)))
    return `#${c.map((v) => v.toString(16).padStart(2, '0')).join('')}`
  }

  const keys = ACCOUNT_COLOR_KEYS
  test.each([
    ['ライト', light],
    ['ダーク', dark],
  ])('%s: 10 色すべての薄い地の上で --ink は 4.5:1 以上', (_, v) => {
    for (const key of keys) {
      expect(
        contrastRatio(v['--ink'], mix(v[`--account-${key}`], v['--surface'])),
      ).toBeGreaterThanOrEqual(4.5)
    }
  })

  test.each([
    ['ライト', light],
    ['ダーク', dark],
  ])('%s: Gmail の極端な色（白・黒）が来ても --ink は 4.5:1 以上', (_, v) => {
    for (const raw of ['#ffffff', '#000000']) {
      expect(contrastRatio(v['--ink'], mix(raw, v['--surface']))).toBeGreaterThanOrEqual(4.5)
    }
  })

  test.each([
    ['ライト', light],
    ['ダーク', dark],
  ])('%s: 一覧に出る青・緑・紫・橙の薄い地は互いに別の色（地の差で見分けられる）', (_, v) => {
    const bgs = ['blue', 'green', 'purple', 'orange'].map((k) =>
      mix(v[`--account-${k}`], v['--surface']),
    )
    for (let i = 0; i < bgs.length; i++) {
      for (let j = i + 1; j < bgs.length; j++) {
        const a = parseHex(bgs[i])
        const b = parseHex(bgs[j])
        const dist = Math.max(Math.abs(a.r - b.r), Math.abs(a.g - b.g), Math.abs(a.b - b.b))
        expect(dist).toBeGreaterThanOrEqual(6)
      }
    }
  })
})
