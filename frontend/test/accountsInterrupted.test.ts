import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  calls,
  callsOf,
  failures,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
  until,
} from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAccountsStore } = await import('../src/stores/accounts')
const { useUiStore } = await import('../src/stores/ui')

beforeEach(resetKit)

describe('認可の中断の案内（Android）', () => {
  test('前回の起動が認可の途中で止められていたら、やり直しを案内するお知らせ（インラインアラート）を 1 回出す', async () => {
    responses.TakeInterruptedAuth = true
    const s = useAccountsStore()
    await s.start()
    const alerts = useUiStore().alerts
    expect(alerts).toHaveLength(1)
    expect(alerts[0].kind).toBe('info')
    expect(alerts[0].message).toContain('やり直して')
    expect(callsOf('TakeInterruptedAuth')).toHaveLength(1)

    await s.start() // 冪等（再度呼ばない）
    expect(callsOf('TakeInterruptedAuth')).toHaveLength(1)
    expect(useUiStore().alerts).toHaveLength(1)
  })

  test('中断が無ければ何も出さない', async () => {
    const s = useAccountsStore()
    await s.start()
    expect(useUiStore().alerts).toHaveLength(0)
    expect(calls.length).toBeGreaterThan(0)
  })

  test('検出の呼び出しが失敗しても起動は続き、お知らせも出さない', async () => {
    failures.TakeInterruptedAuth = new Error('boom')
    const s = useAccountsStore()
    await s.start()
    expect(useUiStore().alerts).toHaveLength(0)
    expect(s.accounts).toEqual([])
  })
})

describe('起動時のアカウント読み込みの失敗', () => {
  test('INTERNAL で失敗したらエラーのアラートを 1 件出し、再試行で読み込み直す', async () => {
    failures.ListAccounts = appError('INTERNAL')
    const s = useAccountsStore()
    const ui = useUiStore()
    await s.start()
    expect(ui.alerts).toHaveLength(1)
    expect(ui.alerts[0].kind).toBe('error')
    expect(callsOf('ListAccounts')).toHaveLength(1)

    delete failures.ListAccounts
    responses.ListAccounts = [
      { id: 'a1', displayName: 'A', email: 'a@example.com', status: 'active' },
    ]
    ui.alerts[0].retry?.()
    await until(() => s.accounts.length === 1)
    expect(callsOf('ListAccounts')).toHaveLength(2)
  })

  test('OFFLINE ではアラートを出さず、共通のオフライン表示に任せる', async () => {
    failures.ListAccounts = appError('OFFLINE')
    failures.ListOAuthClients = appError('OFFLINE')
    const s = useAccountsStore()
    const ui = useUiStore()
    await s.start()
    expect(ui.alerts).toHaveLength(0)
    expect(ui.online).toBe(false)
  })
})
