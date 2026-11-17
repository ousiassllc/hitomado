import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
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
    expect(w.get('[data-testid="inbox-failure-row"]').text()).toBe(`会社A: ${rateLimited.message}`)
    await w.get('[data-testid="inbox-failure-retry"]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
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
    expect(rows).toEqual([`会社A、個人: ${rateLimited.message}`, `チーム: ${other.message}`])
  })

  test('アカウントが見つからないときは名前を代わりの文言にする', () => {
    const { pinia } = setup()
    const w = mount(InboxFailureNotice, {
      props: { failures: [{ ...rateLimited, accountId: 'gone' }] },
      global: { plugins: [pinia] },
    })
    expect(w.text()).toContain('不明なアカウント: ')
  })
})

describe('ThreadListPane の失敗表示（#392）', () => {
  test('案内を出し、「再試行」で inbox.load を 1 回呼ぶ', async () => {
    const { pinia, inbox, loads } = setup()
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.text()).toContain('会社A')
    expect(w.text()).toContain(rateLimited.message)
    await w.get('[data-testid="inbox-failure-retry"]').trigger('click')
    expect(loads()).toBe(1)
  })

  test('全アカウントが失敗して 0 件のとき、通常の空表示でなく失敗の空表示を出す', () => {
    const { pinia, inbox } = setup()
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.text()).not.toContain(EMPTY)
    expect(w.text()).toContain(FAILED_EMPTY)
  })

  test('検索でも同じ（「一致するスレッドがありません」と言わない）', () => {
    const { pinia, inbox } = setup()
    inbox.mode = 'search'
    inbox.failures = [rateLimited]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.text()).not.toContain('一致するスレッドがありません。')
    expect(w.text()).toContain(FAILED_EMPTY)
  })

  test('要再接続だけの失敗は案内の行を出さない（BannerStack が出す）が、空表示は失敗のもの', () => {
    const { pinia, inbox } = setup()
    inbox.failures = [needsReauth]
    const w = mount(ThreadListPane, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="inbox-failure-notice"]').exists()).toBe(false)
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
