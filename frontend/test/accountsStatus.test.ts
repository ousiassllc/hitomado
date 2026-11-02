import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import type { Account } from '../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import AccountRow from '../src/components/settings/AccountRow.vue'
import { installMocks, resetKit, responses, restoreMocks } from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAccountsStore } = await import('../src/stores/accounts')
const { useUiStore } = await import('../src/stores/ui')

beforeEach(resetKit)

const rejected = 'OAuth クライアントの設定（Client ID／Secret）を確認してください'

const account = (id: string, status: string, statusMessage?: string): Account =>
  ({
    id,
    displayName: `名前-${id}`,
    email: `${id}@example.com`,
    color: 'blue',
    notifyEnabled: true,
    status,
    statusMessage,
  }) as Account

describe('accounts ストアの statusMessage', () => {
  test('再起動後（ListAccounts の statusMessage のみ）でも理由を保持する', async () => {
    responses.ListAccounts = [
      account('a1', 'needs_reauth', rejected),
      account('a2', 'active'),
      account('a3', 'needs_reauth', '再接続が必要です'),
    ]
    const s = useAccountsStore()
    await s.load()
    expect(s.statuses.a1).toBe('needs_reauth')
    expect(s.statusMessages).toEqual({ a1: rejected, a3: '再接続が必要です' })
  })

  test('account:status の message でアカウント側の statusMessage も更新され、再取得で巻き戻らない', async () => {
    responses.ListAccounts = [account('a1', 'active')]
    const s = useAccountsStore()
    await s.load()
    s.onAccountStatus({ accountId: 'a1', status: 'needs_reauth', message: rejected })
    expect(s.statusMessages.a1).toBe(rejected)
    expect(s.accounts[0].statusMessage).toBe(rejected)

    // 正常に戻ったら理由は消える
    s.onAccountStatus({ accountId: 'a1', status: 'active' })
    expect(s.statusMessages.a1).toBeUndefined()
  })
})

describe('AccountRow の要再接続表示', () => {
  const mountRow = (status: string, statusMessage?: string) =>
    mount(AccountRow, {
      props: {
        account: account('a1', status, statusMessage),
        index: 0,
        total: 1,
        status,
        statusMessage,
        busy: false,
      },
    })

  test('クライアント拒否は Go のメッセージと「OAuth クライアントを確認」を出す', async () => {
    const w = mountRow('needs_reauth', rejected)
    expect(w.get('.status').text()).toBe(rejected)
    const buttons = w.findAll('button').map((b) => b.text())
    expect(buttons).toContain('OAuth クライアントを確認')
    expect(buttons).toContain('再接続')
    await w
      .findAll('button')
      .find((b) => b.text() === 'OAuth クライアントを確認')
      ?.trigger('click')
    expect(w.emitted('checkClient')).toHaveLength(1)
  })

  test('通常の要再接続は従来の固定文言で、確認ボタンは出ない', () => {
    const w = mountRow('needs_reauth', '再接続が必要です')
    expect(w.get('.status').text()).toBe('再接続が必要です')
    expect(w.findAll('button').map((b) => b.text())).not.toContain('OAuth クライアントを確認')
  })
})

describe('ui.openSettings', () => {
  test('タブを指定でき、省略時はアカウント', () => {
    const ui = useUiStore()
    ui.openSettings('oauth')
    expect(ui.settingsTab).toBe('oauth')
    expect(ui.dialogs.settings).toBe(true)
    ui.openSettings()
    expect(ui.settingsTab).toBe('accounts')
  })
})
