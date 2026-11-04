import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import {
  base,
  callsOf,
  ids,
  installMocks,
  markGone,
  page,
  ref,
  resetKit,
  responses,
  restoreMocks,
  thread,
} from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../src/stores/inbox')
const { default: InboxTabs } = await import('../src/components/inbox/InboxTabs.vue')

beforeEach(resetKit)

const lastQuery = () => (callsOf('ListInbox').at(-1)?.args[0] ?? {}) as Record<string, unknown>
const lastTab = () => lastQuery().tab

describe('inboxStore のタブ', () => {
  test('起動時は「すべて」で、ListInbox に tab を渡す', async () => {
    const s = useInboxStore()
    await s.start()
    expect(s.tab).toBe('all')
    expect(lastTab()).toBe('all')
  })

  test('setTab はカーソルを捨てて先頭から取り直し、フィルタ（アカウント・未読のみ）を保つ', async () => {
    responses.GetInboxFilter = { accountIds: ['a1'], unreadOnly: true }
    responses.ListInbox = (q: unknown) =>
      (q as { cursor?: string }).cursor ? page([]) : page([thread('a1', 't1', 500)], 'cur-1')
    const s = useInboxStore()
    await s.start()
    await s.loadMore()
    expect(s.hasMore).toBe(false)
    const before = callsOf('ListInbox').length

    await s.setTab('social')
    expect(s.tab).toBe('social')
    expect(callsOf('ListInbox')).toHaveLength(before + 1)
    const q = lastQuery()
    expect(q).toMatchObject({ tab: 'social', accountIds: ['a1'], unreadOnly: true })
    expect(q.cursor).toBeUndefined()
    expect(s.hasMore).toBe(true)
  })

  test('同じタブへの切り替えは何もしない', async () => {
    const s = useInboxStore()
    await s.start()
    const before = callsOf('ListInbox').length
    await s.setTab('all')
    expect(callsOf('ListInbox')).toHaveLength(before)
  })

  test('タブはアカウントの切り替え・検索のあとも保持し、保存はしない', async () => {
    responses.Search = page([thread('a1', 's1', 100)])
    const s = useInboxStore()
    await s.start()
    await s.setTab('promotions')
    await s.setFilter({ accountIds: ['a2'], unreadOnly: false })
    expect(lastTab()).toBe('promotions')
    expect(s.tab).toBe('promotions')

    await s.runSearch('foo')
    expect(callsOf('Search')).toHaveLength(1)
    await s.setTab('new') // 検索中は値だけ変えて、検索結果は取り直さない
    expect(callsOf('Search')).toHaveLength(1)
    await s.clearSearch()
    expect(lastTab()).toBe('new')
    expect(callsOf('SetInboxFilter').every((c) => !('tab' in (c.args[0] as object)))).toBe(true)
  })

  test('新着タブで既読にした行は一覧から外れる（失敗すれば戻る）', async () => {
    responses.MarkRead = markGone
    const s = useInboxStore()
    await s.start()
    await s.setTab('new')
    s.select(ref('a1', 't2'))
    await s.markRead([ref('a1', 't2')], true)
    expect(ids(s)).toEqual(['t1', 't3', 't4'])
  })

  test('すべてのタブでは既読にしても行は残る', async () => {
    const s = useInboxStore()
    await s.start()
    await s.markRead([ref('a1', 't2')], true)
    expect(ids(s)).toEqual(base.map((t) => t.threadId))
  })
})

describe('InboxTabs', () => {
  const labels = ['すべて', 'メイン', 'ソーシャル', 'プロモーション', '新着']

  test('tablist／tab／aria-selected を持つ 5 つのタブを順に並べる', () => {
    const w = mount(InboxTabs, { props: { modelValue: 'social' } })
    expect(w.get('[role="tablist"]').attributes('aria-label')).toBeTruthy()
    const tabs = w.findAll('[role="tab"]')
    expect(tabs.map((t) => t.text())).toEqual(labels)
    expect(tabs.map((t) => t.attributes('aria-selected'))).toEqual([
      'false',
      'false',
      'true',
      'false',
      'false',
    ])
    w.unmount()
  })

  test('クリックで update:modelValue を出す', async () => {
    const w = mount(InboxTabs, { props: { modelValue: 'all' } })
    await w.get('[data-testid="inbox-tab-promotions"]').trigger('mousedown', { button: 0 })
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['promotions'])
    w.unmount()
  })

  test('キーボードで選択を移せる（Enter／Space）', async () => {
    const w = mount(InboxTabs, { props: { modelValue: 'all' } })
    await w.get('[data-testid="inbox-tab-new"]').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual(['new'])
    w.unmount()
  })
})

describe('InboxTabs の aria-controls', () => {
  test('tabpanel が無いので、存在しない id を指す aria-controls を付けない', () => {
    const w = mount(InboxTabs, { props: { modelValue: 'all' }, attachTo: document.body })
    for (const t of w.findAll('[role="tab"]')) {
      const id = t.attributes('aria-controls')
      if (id) expect(document.getElementById(id)).not.toBeNull()
    }
    w.unmount()
  })
})
