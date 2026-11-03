import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  calls,
  callsOf,
  failures,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAccountsStore } = await import('../src/stores/accounts')
const { useUiStore } = await import('../src/stores/ui')

beforeEach(resetKit)

describe('認可の中断の案内（Android）', () => {
  test('前回の起動が認可の途中で止められていたら、やり直しを案内するトーストを 1 回出す', async () => {
    responses.TakeInterruptedAuth = true
    const s = useAccountsStore()
    await s.start()
    const toasts = useUiStore().toasts
    expect(toasts).toHaveLength(1)
    expect(toasts[0].kind).toBe('info')
    expect(toasts[0].message).toContain('やり直して')
    expect(callsOf('TakeInterruptedAuth')).toHaveLength(1)

    await s.start() // 冪等（再度呼ばない）
    expect(callsOf('TakeInterruptedAuth')).toHaveLength(1)
    expect(useUiStore().toasts).toHaveLength(1)
  })

  test('中断が無ければ何も出さない', async () => {
    const s = useAccountsStore()
    await s.start()
    expect(useUiStore().toasts).toHaveLength(0)
    expect(calls.length).toBeGreaterThan(0)
  })

  test('検出の呼び出しが失敗しても起動は続き、トーストも出さない', async () => {
    failures.TakeInterruptedAuth = new Error('boom')
    const s = useAccountsStore()
    await s.start()
    expect(useUiStore().toasts).toHaveLength(0)
    expect(s.accounts).toEqual([])
  })
})
