import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { appError, base, failures, installMocks, ref, resetKit, responses, restoreMocks } from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../src/stores/inbox')
const { useUiStore } = await import('../src/stores/ui')
const { useAccountsStore } = await import('../src/stores/accounts')

beforeEach(resetKit)

describe('オフライン・要再接続', () => {
  test('OFFLINE で online=false、次の成功で復帰する', async () => {
    failures.ListInbox = appError('OFFLINE')
    failures.GetUnreadCounts = appError('OFFLINE')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    expect(ui.online).toBe(false)
    expect(ui.toasts).toHaveLength(0)
    delete failures.ListInbox
    delete failures.GetUnreadCounts
    await s.load()
    expect(ui.online).toBe(true)
  })

  test('NEEDS_REAUTH でアカウント状態を更新する', async () => {
    failures.Archive = appError('NEEDS_REAUTH', { accountId: 'a2' })
    const s = useInboxStore()
    const accounts = useAccountsStore()
    await s.load()
    await s.archive([ref('a2', 't3')])
    expect(accounts.statuses.a2).toBe('needs_reauth')
  })

  test('ページの部分失敗（NEEDS_REAUTH）でもアカウント状態を更新する', async () => {
    responses.ListInbox = {
      items: base,
      failures: [{ accountId: 'a2', error: { code: 'NEEDS_REAUTH', message: 'x', retryable: false } }],
    }
    const s = useInboxStore()
    const accounts = useAccountsStore()
    await s.load()
    expect(accounts.statuses.a2).toBe('needs_reauth')
    expect(s.failures[0].accountId).toBe('a2')
  })
})

describe('キーボード選択', () => {
  test('moveCursor は端で止まり、extend で範囲選択する', async () => {
    const s = useInboxStore()
    await s.load()
    s.moveCursor(1)
    expect(s.cursorKey).toBe('a1:t1')
    s.moveCursor(1)
    expect(s.cursorKey).toBe('a1:t2')
    s.moveCursor(1, true)
    expect(s.checkedKeys).toEqual(['a1:t2', 'a2:t3'])
    s.moveCursor(-10)
    expect(s.cursorKey).toBe('a1:t1')
    s.openCursor()
    expect(s.selectedKey).toBe('a1:t1')
    expect(s.checkedKeys).toEqual([])
  })
})
