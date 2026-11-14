import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import * as realCompose from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/composeservice'
import { callsOf, installMocks, resetKit, responses, restoreMocks } from '../../../test/inboxKit'

installMocks()
// 下書きの一覧（#366）を開く入口を試すため、ListDrafts だけ差し替える（ほかは本物）。
const COMPOSE = '../../../bindings/github.com/ousiassllc/hitomado/internal/app/composeservice'
const listDrafts: unknown[] = []
mock.module(COMPOSE, () => ({
  ...realCompose,
  ListDrafts: async (ids: string[]) => {
    listDrafts.push(ids)
    return { items: [], failures: [], truncated: false }
  },
}))
afterAll(() => {
  restoreMocks()
  mock.module(COMPOSE, () => ({ ...realCompose }))
})

const { useInboxStore } = await import('../../stores/inbox')
const { useAccountsStore } = await import('../../stores/accounts')
const { useUiStore } = await import('../../stores/ui')
const { useDraftsStore } = await import('../../stores/drafts')
const { default: BottomTabs } = await import('./BottomTabs.vue')
const { default: SidebarPlaces } = await import('./SidebarPlaces.vue')
const { default: AgentPanel } = await import('./AgentPanel.vue')
const { default: AccountChips } = await import('../account/AccountChips.vue')

beforeEach(() => {
  resetKit()
  listDrafts.length = 0
  useDraftsStore().hide()
})

const account = (id: string, name: string) =>
  ({ id, email: `${id}@example.com`, displayName: name, color: 'blue', signatureHtml: '' }) as never

async function ready() {
  responses.GetUnreadCounts = { a1: 3, a2: 4 }
  const inbox = useInboxStore()
  const accounts = useAccountsStore()
  accounts.accounts = [account('a1', '仕事'), account('a2', '私用')]
  await inbox.load()
  return { inbox, ui: useUiStore() }
}

describe('サイドバーの見る場所', () => {
  test('受信箱・下書き・送信済み・アーカイブの順で、未読数は受信箱だけに付き、押すと見る場所が変わる', async () => {
    const { inbox } = await ready()
    const w = mount(SidebarPlaces)
    expect(w.findAll('button.place').map((b) => b.text().replace(/\d+$/, ''))).toEqual([
      '受信箱',
      '下書き',
      '送信済み',
      'アーカイブ',
    ])
    expect(w.findAll('.badge').map((b) => b.text())).toEqual(['7'])
    expect(w.get('[data-testid=nav-inbox]').attributes('aria-current')).toBe('page')

    await w.get('[data-testid=nav-sent]').trigger('click')
    await flushPromises()
    expect(inbox.view).toBe('sent')
    expect(w.get('[data-testid=nav-sent]').attributes('aria-current')).toBe('page')
    expect(w.get('[data-testid=nav-inbox]').attributes('aria-current')).toBeUndefined()
  })

  test('「下書き」は準備中ではなく下書きの一覧を開く。ほかの場所を押すと一覧を閉じてその場所へ移る（#366）', async () => {
    const { inbox } = await ready()
    const drafts = useDraftsStore()
    const w = mount(SidebarPlaces)

    await w.get('[data-testid=nav-drafts]').trigger('click')
    await flushPromises()
    expect(drafts.active).toBe(true)
    expect(listDrafts).toHaveLength(1)
    expect(w.get('[data-testid=nav-drafts]').attributes('aria-current')).toBe('page')
    expect(w.get('[data-testid=nav-inbox]').attributes('aria-current')).toBeUndefined()

    await w.get('[data-testid=nav-archive]').trigger('click')
    await flushPromises()
    expect(drafts.active).toBe(false)
    expect(inbox.view).toBe('archive')
    expect(w.get('[data-testid=nav-archive]').attributes('aria-current')).toBe('page')
    expect(w.get('[data-testid=nav-drafts]').attributes('aria-current')).toBeUndefined()

    await w.get('[data-testid=nav-drafts]').trigger('click')
    await w.get('[data-testid=nav-inbox]').trigger('click')
    await flushPromises()
    expect(drafts.active).toBe(false)
    expect(inbox.view).toBe('inbox')
  })
})

describe('スマホの下 4 タブ', () => {
  test('受信箱・下書き・エージェント・その他。受信箱だけに未読バッジ。エージェントはパネルを開く', async () => {
    const { inbox, ui } = await ready()
    const w = mount(BottomTabs)
    expect(w.findAll('.tab').map((t) => t.text().replace(/^\d+ /, ''))).toEqual([
      '受信箱',
      '下書き',
      'エージェント',
      'その他',
    ])
    expect(w.get('[data-testid=tab-inbox] .badge').text()).toBe('7')
    expect(w.findAll('.badge')).toHaveLength(1)

    await w.get('[data-testid=tab-drafts]').trigger('click')
    await flushPromises()
    expect(useDraftsStore().active).toBe(true)
    expect(listDrafts).toHaveLength(1)
    expect(w.get('[data-testid=tab-drafts]').attributes('aria-current')).toBe('page')
    expect(w.get('[data-testid=tab-inbox]').attributes('aria-current')).toBeUndefined()

    await w.get('[data-testid=tab-agent]').trigger('click')
    expect(ui.agentOpen).toBe(true)
    expect(w.get('[data-testid=tab-agent]').attributes('aria-expanded')).toBe('true')
    expect(w.get('[data-testid=tab-drafts]').attributes('aria-current')).toBeUndefined()

    // 場所のタブを押すとエージェントのパネルは閉じ、下書きの一覧も閉じて受信箱へ戻る
    await w.get('[data-testid=tab-inbox]').trigger('click')
    expect(ui.agentOpen).toBe(false)
    expect(useDraftsStore().active).toBe(false)
    expect(inbox.view).toBe('inbox')
    expect(w.get('[data-testid=tab-inbox]').attributes('aria-current')).toBe('page')
  })
})

describe('スマホのアカウントのチップ', () => {
  test('先頭は「全アカウント」。押したチップを加える／外し、見る場所と未読のみは変えない', async () => {
    const { inbox } = await ready()
    await inbox.setView('sent')
    const w = mount(AccountChips)
    const chips = w.findAll('button.chip')
    expect(chips.map((c) => c.text())).toEqual(['全アカウント', '仕事', '私用'])
    expect(chips[0]?.attributes('aria-pressed')).toBe('true')

    await chips[1]?.trigger('click')
    await chips[2]?.trigger('click')
    await flushPromises()
    expect(inbox.filter.accountIds).toEqual(['a1', 'a2'])
    expect(inbox.view).toBe('sent')
    const last = callsOf('Search').at(-1)?.args[0] as { q: string; accountIds: string[] }
    expect(last.q).toBe('in:sent')
    expect(last.accountIds).toEqual(['a1', 'a2'])

    await chips[1]?.trigger('click')
    await flushPromises()
    expect(inbox.filter.accountIds).toEqual(['a2'])
    await chips[0]?.trigger('click')
    await flushPromises()
    expect(inbox.filter.accountIds).toEqual([])
  })
})

describe('エージェントのパネル', () => {
  test('中身は準備中の表示だけ。Esc で閉じる', async () => {
    const ui = useUiStore()
    ui.openAgent()
    const w = mount(AgentPanel, { attachTo: document.body })
    expect(w.text()).toContain('準備中')
    await w.get('aside').trigger('keydown', { key: 'Escape' })
    expect(ui.agentOpen).toBe(false)
    w.unmount()
  })
})
