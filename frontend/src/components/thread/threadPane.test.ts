import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { callsOf, ctl, detail, installMocks, REF, resetState, restoreMocks } from './threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../stores/thread')
const { default: ThreadPane } = await import('./ThreadPane.vue')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})

function multi() {
  const base = detail(false).messages?.[0]
  if (!base) throw new Error('fixture')
  return detail(false, {
    messages: [
      { ...base, id: 'm1', bodyText: '古い本文' },
      { ...base, id: 'm2', isUnread: true, bodyText: '未読の本文' },
      { ...base, id: 'm3', bodyText: '中間の本文' },
      { ...base, id: 'm4', bodyText: '最新の本文' },
    ],
    draftId: 'd1',
  })
}

describe('ThreadPane', () => {
  test('未選択のときは空状態を出す', () => {
    const w = mount(ThreadPane)
    expect(w.find('[data-testid="thread-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="thread-header"]').exists()).toBe(false)
  })

  test('ヘッダに件名と受信アカウント（名前・アドレス）を常時表示する', async () => {
    ctl.responses.GetThread = detail(false)
    const store = useThreadStore()
    const w = mount(ThreadPane)
    await store.open(REF)
    await w.vm.$nextTick()
    const header = w.get('[data-testid="thread-header"]')
    expect(header.get('[data-testid="thread-subject"]').text()).toBe('件名')
    expect(header.text()).toContain('個人')
    expect(header.text()).toContain('me@example.com')
  })

  test('最新と未読のメッセージだけ展開され、他は折りたたまれる', async () => {
    ctl.responses.GetThread = multi()
    const store = useThreadStore()
    const w = mount(ThreadPane)
    await store.open(REF)
    await w.vm.$nextTick()
    const text = w.get('[data-testid="thread-scroll"]').text()
    expect(text).toContain('未読の本文')
    expect(text).toContain('最新の本文')
    expect(text).not.toContain('古い本文')
    expect(text).not.toContain('中間の本文')
  })

  test('下書きがあれば「下書きを開く」を出す。返信エディタ未使用時は ReplyBar を表示する', async () => {
    ctl.responses.GetThread = multi()
    const store = useThreadStore()
    const w = mount(ThreadPane)
    await store.open(REF)
    await w.vm.$nextTick()
    expect(w.find('[data-testid="reply-bar"]').exists()).toBe(true)
    expect(w.find('[data-testid="open-draft"]').exists()).toBe(true)
  })

  test('ヘッダの「未読にする」で markRead(false) が呼ばれる', async () => {
    ctl.responses.GetThread = detail(false)
    const store = useThreadStore()
    const w = mount(ThreadPane)
    await store.open(REF)
    await w.vm.$nextTick()
    await w.get('[data-testid="act-toggle-read"]').trigger('click')
    await Promise.resolve()
    expect(callsOf('MarkRead').at(-1)?.args).toEqual([[REF], false])
  })

  test('読み込みエラーでは再試行ボタンを出す', async () => {
    ctl.failures.GetThread = {
      cause: JSON.stringify({ code: 'GMAIL_ERROR', message: '失敗', retryable: true }),
    }
    const store = useThreadStore()
    const w = mount(ThreadPane)
    await store.open(REF)
    await w.vm.$nextTick()
    expect(w.get('[data-testid="thread-error"]').text()).toContain('失敗')
  })
})
