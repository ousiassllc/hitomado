import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import type { DraftSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import {
  calls,
  h,
  installMocks,
  reset,
  restoreMocks,
  session,
  setup,
  useDraftsStore,
} from '../../stores/__tests__/composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(() => {
  reset()
  document.body.innerHTML = ''
})

const { default: DraftList } = await import('./DraftList.vue')
const { default: SidebarViews } = await import('../shell/SidebarViews.vue')

function draft(over: Partial<DraftSummary> = {}): DraftSummary {
  return {
    accountId: 'a1',
    draftId: 'd1',
    threadId: 'th1',
    updatedAt: Date.UTC(2026, 10, 14, 3, 0),
    to: [{ name: '花子', email: 'hanako@example.com' }],
    subject: '打ち合わせの日程',
    snippet: '来週の火曜か木曜でいかがでしょうか',
    isReply: false,
    ...over,
  }
}

async function mountList(items: DraftSummary[]) {
  const ctx = setup()
  h.responses.ListDrafts = { items, failures: [], truncated: false }
  const drafts = useDraftsStore()
  const w = mount(DraftList, { attachTo: document.body })
  drafts.show()
  await flushPromises()
  return { w, drafts, ...ctx }
}

const dialogButton = (label: string) =>
  Array.from(document.body.querySelectorAll('button')).find((b) => b.textContent?.trim() === label)

describe('DraftList（下書きの一覧。#366）', () => {
  test('行には、アカウントの識別チップ・宛先・件名・本文の先頭を出す。宛先・件名・本文が無い下書きは、その旨を出す', async () => {
    const { w } = await mountList([
      draft(),
      draft({ draftId: 'd2', accountId: 'a2', to: [], subject: '', snippet: '' }),
    ])
    const rows = w.findAll('li')
    expect(rows).toHaveLength(2)
    expect(rows[0].find('.account-chip').text()).toContain('A')
    expect(rows[0].text()).toContain('花子')
    expect(rows[0].text()).toContain('打ち合わせの日程')
    expect(rows[0].text()).toContain('来週の火曜か木曜でいかがでしょうか')
    expect(rows[1].find('.account-chip').text()).toContain('B')
    expect(rows[1].text()).toContain('(宛先なし)')
    expect(rows[1].text()).toContain('(件名なし)')
    expect(rows[1].text()).toContain('(本文なし)')
    expect(rows[0].get('button.open').attributes('aria-label')).toBe('A 花子、打ち合わせの日程')
    w.unmount()
  })

  test('行を押すと、その下書きを開く。開いている作成画面の下書きの行は現在行になる', async () => {
    const { w, compose } = await mountList([draft(), draft({ draftId: 'd2', accountId: 'a2' })])
    h.responses.OpenCompose = session({
      mode: 'new',
      senderLocked: false,
      accountId: 'a1',
      draftId: 'd1',
    })
    await w.findAll('button.open')[0].trigger('click')
    await flushPromises()
    expect(calls.find((c) => c.name === 'OpenCompose')?.args[0]).toEqual({
      mode: 'new',
      accountId: 'a1',
      draftId: 'd1',
    })
    expect(compose.session?.draftId).toBe('d1')
    const rows = w.findAll('li')
    expect(rows[0].classes()).toContain('selected')
    expect(rows[0].get('button.open').attributes('aria-current')).toBe('true')
    expect(rows[1].classes()).not.toContain('selected')
    w.unmount()
  })

  test('「破棄」は確認のモーダルを挟み、確認するまで消さない。キャンセルしても消えない', async () => {
    const { w, drafts } = await mountList([draft(), draft({ draftId: 'd2' })])
    h.responses.DiscardDraft = undefined
    await w.findAll('button.discard')[0].trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('この下書きを破棄しますか？')
    expect(document.body.textContent).toContain('Gmail に保存済みの下書きが削除されます')
    expect(calls.filter((c) => c.name === 'DiscardDraft')).toHaveLength(0)

    dialogButton('キャンセル')?.click()
    await flushPromises()
    expect(calls.filter((c) => c.name === 'DiscardDraft')).toHaveLength(0)
    expect(drafts.items).toHaveLength(2)

    await w.findAll('button.discard')[0].trigger('click')
    await flushPromises()
    dialogButton('破棄する')?.click()
    await flushPromises()
    expect(calls.filter((c) => c.name === 'DiscardDraft')).toHaveLength(1)
    expect(drafts.items.map((d) => d.draftId)).toEqual(['d2'])
    expect(w.findAll('li')).toHaveLength(1)
    w.unmount()
  })

  test('0 件のときは空の案内。読み込み中はスピナー。切り捨てたときは注記', async () => {
    const empty = await mountList([])
    expect(empty.w.text()).toContain('下書きはありません。')
    empty.w.unmount()

    setup()
    h.responses.ListDrafts = { items: [draft()], failures: [], truncated: true }
    const w = mount(DraftList, { attachTo: document.body })
    useDraftsStore().show()
    await flushPromises()
    expect(w.get('.note').text()).toBe('各アカウントの新しい 100 件までを表示しています。')
    w.unmount()
  })
})

describe('SidebarViews（サイドバーの入口）', () => {
  test('「下書き」で一覧を出し、「受信箱」で戻る。いまの表示が aria-current になる', async () => {
    setup()
    h.responses.ListDrafts = { items: [], failures: [], truncated: false }
    const drafts = useDraftsStore()
    const w = mount(SidebarViews)
    expect(w.get('[data-testid="nav-inbox"]').attributes('aria-current')).toBe('page')
    await w.get('[data-testid="nav-drafts"]').trigger('click')
    await flushPromises()
    expect(drafts.active).toBe(true)
    expect(w.get('[data-testid="nav-drafts"]').attributes('aria-current')).toBe('page')
    expect(w.get('[data-testid="nav-inbox"]').attributes('aria-current')).toBeUndefined()
    await w.get('[data-testid="nav-inbox"]').trigger('click')
    expect(drafts.active).toBe(false)
  })
})
