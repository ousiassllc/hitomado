import { afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import type { ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import BaseButton from '../ui/BaseButton.vue'
import ThreadList from './ThreadList.vue'

setActivePinia(createPinia())

const thread: ThreadSummary = {
  accountId: 'a1',
  threadId: 't1',
  subject: '見積のご確認',
  snippet: 'お世話になっております',
  lastMessageAt: Date.now(),
  lastFrom: { name: '山田太郎', email: 'yamada@example.com' },
  messageCount: 1,
  isUnread: false,
  hasAttachment: false,
  inInbox: true,
  labels: [],
}

// ガイド O7: 続きの読み込みは無限スクロールにせず、明示のボタンだけにする。
describe('ThreadList の続きの読み込みは明示ボタンだけ（O7）', () => {
  const items = Array.from({ length: 3 }, (_, i) => ({ ...thread, threadId: `t${i}` }))
  const props = {
    items,
    checkedKeys: [],
    loading: false,
    hasMore: false,
    loadingMore: false,
    emptyText: '空',
  }

  test('末尾の行が見えていて続きがあっても、loadMore を自動で通知しない', async () => {
    const w = mount(ThreadList, { props })
    await w.setProps({ hasMore: true })
    await nextTick()
    expect(w.emitted('loadMore')).toBeUndefined()
  })

  test('「さらに読み込む」はサブ（secondary）・小（sm）のボタン', () => {
    const w = mount(ThreadList, { props: { ...props, hasMore: true } })
    const btn = w.getComponent(BaseButton)
    expect(btn.props('variant')).toBe('secondary')
    expect(btn.props('size')).toBe('sm')
  })
})

// a11y（#337）: listbox の子は option だけにし、操作ボタン・状態表示は listbox の外（同じスクロール領域の兄弟）に置く。
describe('ThreadList の listbox 構造（#337）', () => {
  const items = Array.from({ length: 3 }, (_, i) => ({ ...thread, threadId: `t${i}` }))
  const props = {
    items,
    checkedKeys: [],
    loading: false,
    hasMore: true,
    loadingMore: false,
    emptyText: '空',
  }

  test('「さらに読み込む」は listbox の外にあり、listbox の子孫に button が無い', () => {
    const w = mount(ThreadList, { props })
    const listbox = w.get('[role="listbox"]')
    expect(listbox.find('button').exists()).toBe(false)
    expect(w.find('.more button').exists()).toBe(true)
    expect(listbox.element.contains(w.get('.more').element)).toBe(false)
    expect(listbox.element.parentElement?.classList.contains('list')).toBe(true)
    expect(w.get('.more').element.parentElement?.classList.contains('list')).toBe(true)
  })

  test('空表示・読み込み中も listbox の外に置く（listbox は行があるときだけ）', async () => {
    const empty = mount(ThreadList, { props: { ...props, items: [], hasMore: false } })
    expect(empty.find('[role="listbox"]').exists()).toBe(false)
    expect(empty.get('.state').text()).toContain('空')
    const loading = mount(ThreadList, {
      props: { ...props, items: [], hasMore: false, loading: true },
    })
    expect(loading.find('[role="listbox"]').exists()).toBe(false)
    expect(loading.find('.state').exists()).toBe(true)
  })

  test('listbox が tabindex・aria-activedescendant・矢印キー処理を持つ', async () => {
    const w = mount(ThreadList, { props: { ...props, cursorKey: 'a1:t1' } })
    const listbox = w.get('[role="listbox"]')
    expect(listbox.attributes('tabindex')).toBe('0')
    expect(listbox.attributes('aria-activedescendant')).toBeDefined()
    await listbox.trigger('keydown', { key: 'ArrowDown' })
    await listbox.trigger('keydown', { key: 'ArrowUp', shiftKey: true })
    expect(w.emitted('move')).toEqual([
      [1, false],
      [-1, true],
    ])
    // ボタンのキー操作は listbox へ流れない（listbox の外にあるため）
    await w.get('.more button').trigger('keydown', { key: 'ArrowDown' })
    expect(w.emitted('move')).toHaveLength(2)
  })

  // 仮想スクロールはスクロール領域の高さが 0 だと行を描かないので、高さを持たせる。
  const descriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight')
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, value: 600 })
  })
  afterEach(() => {
    if (descriptor) Object.defineProperty(HTMLElement.prototype, 'clientHeight', descriptor)
    else Reflect.deleteProperty(HTMLElement.prototype, 'clientHeight')
  })

  test('各 option が位置（aria-posinset）と件数（aria-setsize）を持つ。続きがあれば件数は不明（-1）', async () => {
    const w = mount(ThreadList, { props })
    await nextTick()
    const options = w.findAll('[role="option"]')
    expect(options.map((o) => o.attributes('aria-posinset'))).toEqual(['1', '2', '3'])
    expect(options.map((o) => o.attributes('aria-setsize'))).toEqual(['-1', '-1', '-1'])
    await w.setProps({ hasMore: false })
    expect(w.findAll('[role="option"]').map((o) => o.attributes('aria-setsize'))).toEqual([
      '3',
      '3',
      '3',
    ])
  })

  test('行のクリックで open を通知する', async () => {
    const w = mount(ThreadList, { props })
    await nextTick()
    await w.findAll('[role="option"]')[1].trigger('click')
    expect(w.emitted('open')?.[0]).toEqual([{ accountId: 'a1', threadId: 't1' }])
  })
})
