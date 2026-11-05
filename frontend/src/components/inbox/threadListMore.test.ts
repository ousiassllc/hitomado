import { describe, expect, test } from 'bun:test'
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
