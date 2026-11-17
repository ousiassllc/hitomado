import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useAccountsStore } from '../../../stores/accounts'
import { useInboxStore } from '../../../stores/inbox'
import ThreadListPane from '../ThreadListPane.vue'
import InboxFailureNotice from './InboxFailureNotice.vue'

const rateLimited = {
  accountId: 'a1',
  code: 'RATE_LIMITED',
  message: 'Gmail の利用制限に達しました。しばらくしてからやり直してください。',
}
const needsReauth = { accountId: 'a2', code: 'NEEDS_REAUTH', message: '再接続が必要です。' }
const item = {
  accountId: 'a1',
  threadId: 't1',
  subject: '件名',
  snippet: '',
  lastMessageAt: 0,
  lastFrom: { name: '田中', email: 't@example.org' },
  messageCount: 1,
  isUnread: false,
  hasAttachment: false,
  inInbox: true,
  labels: [],
} as unknown as ThreadSummary
const EMPTY = '表示するスレッドがありません。'
const FAILED_EMPTY = 'メールを取得できませんでした'

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const accounts = useAccountsStore()
  accounts.accounts = [
    { id: 'a1', displayName: '会社A', email: 'a@example.com', color: 'teal', status: 'active' },
  ] as typeof accounts.accounts
  const inbox = useInboxStore()
  let loads = 0
  inbox.load = async () => {
    loads++
  }
  return { pinia, inbox, loads: () => loads }
}

describe('InboxFailureNotice（#392）', () => {
  test('アカウント名と理由を出し、「再試行」で retry を通知する', async () => {
    const { pinia } = setup()
    const w = mount(InboxFailureNotice, {
      props: { failures: [rateLimited] },
      global: { plugins: [pinia] },
    })
    expect(w.attributes('role')).toBe('alert')
    expect(w.get('h3').text()).toBe('取得できなかったアカウントがあります')
    expect(w.get('[data-testid="inbox-failure-row"]').text()).toBe(`会社A：${rateLimited.message}`)
    await w.get('[data-testid="inbox-failure-retry"]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  test('「再試行」は本文（理由の一覧）の下にある', () => {
    const { pinia } = setup()
    const w = mount(InboxFailureNotice, {
      props: { failures: [rateLimited] },
      global: { plugins: [pinia] },
    })
    const rows = w.get('.reasons').element
    const retry = w.get('[data-testid="inbox-failure-retry"]').element
    expect(rows.compareDocumentPosition(retry) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  test('同じ理由のアカウントは 1 行にまとめ、違う理由は別の行にする', () => {
    const { pinia } = setup()
    const accounts = useAccountsStore()
    accounts.accounts = [
      ...accounts.accounts,
      { id: 'a2', displayName: '個人', email: 'b@example.com', color: 'indigo', status: 'active' },
      { id: 'a3', displayName: 'チーム', email: 'c@example.com', color: 'amber', status: 'active' },
    ] as typeof accounts.accounts
    const other = {
      accountId: 'a3',
      code: 'GMAIL_ERROR',
      message: 'Gmail でエラーが発生しました。',
    }
    const w = mount(InboxFailureNotice, {
      props: { failures: [rateLimited, { ...rateLimited, accountId: 'a2' }, other] },
      global: { plugins: [pinia] },
    })
    const rows = w.findAll('[data-testid="inbox-failure-row"]').map((r) => r.text())
    expect(rows).toEqual([`会社A、個人：${rateLimited.message}`, `チーム：${other.message}`])
  })

  test('アカウントが見つからないときは名前を代わりの文言にする', () => {
    const { pinia } = setup()
    const w = mount(InboxFailureNotice, {
      props: { failures: [{ ...rateLimited, accountId: 'gone' }] },
      global: { plugins: [pinia] },
    })
    expect(w.text()).toContain('不明なアカウント：')
  })
})

describe('ThreadListPane の失敗表示（#392）', () => {
  test('一部だけ失敗（取れた分がある）なら、上に案内を出し、「再試行」で inbox.load を 1 回呼ぶ', async () => {
    const { pinia, inbox, loads } = setup()
    inbox.items = [item]
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(true)
    expect(w.text()).toContain(`会社A：${rateLimited.message}`)
    expect(w.text()).not.toContain(FAILED_EMPTY)
    await w.get('[data-testid="inbox-failure-retry"]').trigger('click')
    expect(loads()).toBe(1)
  })

  test('全アカウントが失敗して 0 件のとき、上の案内は出さず、失敗の空表示の中に「再試行」を置く', async () => {
    const { pinia, inbox, loads } = setup()
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(false)
    expect(w.text()).not.toContain(EMPTY)
    expect(w.text()).toContain(FAILED_EMPTY)
    const retry = w.get('.empty-state [data-testid="inbox-failure-retry"]')
    expect(retry.text()).toBe('再試行')
    await retry.trigger('click')
    expect(loads()).toBe(1)
  })

  test('検索でも同じ（「一致するスレッドがありません」と言わず、空表示に「再試行」）', () => {
    const { pinia, inbox } = setup()
    inbox.mode = 'search'
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(false)
    expect(w.text()).not.toContain('一致するスレッドがありません。')
    expect(w.text()).toContain(FAILED_EMPTY)
    expect(w.find('.empty-state [data-testid="inbox-failure-retry"]').exists()).toBe(true)
  })

  test('要再接続だけの失敗は案内の行を出さない（BannerStack が出す）が、空表示は失敗のもの', () => {
    const { pinia, inbox } = setup()
    inbox.failures = [needsReauth]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(false)
    expect(w.find('[data-testid="inbox-failure-retry"]').exists()).toBe(false)
    expect(w.text()).not.toContain(EMPTY)
    expect(w.text()).toContain(FAILED_EMPTY)
  })

  test('失敗がなければ案内も失敗の空表示も出さない', () => {
    const { pinia } = setup()
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(false)
    expect(w.text()).toContain(EMPTY)
  })
})
