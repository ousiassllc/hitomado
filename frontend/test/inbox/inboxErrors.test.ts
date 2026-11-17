import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  base,
  callsOf,
  failures,
  installMocks,
  page,
  ref,
  resetKit,
  responses,
  restoreMocks,
  until,
} from '../inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../../src/stores/inbox')
const { useUiStore } = await import('../../src/stores/ui')
const { useAccountsStore } = await import('../../src/stores/accounts')

beforeEach(resetKit)

describe('オフライン・要再接続', () => {
  test('OFFLINE で online=false、次の成功で復帰する', async () => {
    failures.ListInbox = appError('OFFLINE')
    failures.GetUnreadCounts = appError('OFFLINE')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    expect(ui.online).toBe(false)
    expect(ui.alerts).toHaveLength(0)
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
      failures: [
        { accountId: 'a2', error: { code: 'NEEDS_REAUTH', message: 'x', retryable: false } },
      ],
    }
    const s = useInboxStore()
    const accounts = useAccountsStore()
    await s.load()
    expect(accounts.statuses.a2).toBe('needs_reauth')
    expect(s.failures[0].accountId).toBe('a2')
  })
})

const rateLimited = {
  accountId: 'a2',
  error: { code: 'RATE_LIMITED', message: 'Gmail の利用制限に達しました。', retryable: true },
}

describe('ページの部分失敗（RATE_LIMITED。#392）', () => {
  test('一部の行があっても failures に残り、アカウント状態・オンライン・アラートは変えない', async () => {
    responses.ListInbox = { items: base.slice(0, 2), failures: [rateLimited] }
    const s = useInboxStore()
    const ui = useUiStore()
    const accounts = useAccountsStore()
    await s.load()
    expect(s.items).toHaveLength(2)
    expect(s.failures).toEqual([
      { accountId: 'a2', code: 'RATE_LIMITED', message: 'Gmail の利用制限に達しました。' },
    ])
    expect(accounts.statuses.a2).toBeUndefined()
    expect(ui.online).toBe(true)
    expect(ui.alerts).toHaveLength(0)
  })

  test('0 件でも failures に残る（検索）', async () => {
    responses.Search = { items: [], failures: [rateLimited] }
    const s = useInboxStore()
    await s.runSearch('from:x')
    expect(s.items).toHaveLength(0)
    expect(s.failures.map((f) => f.code)).toEqual(['RATE_LIMITED'])
  })

  test('次の成功した取得で failures が空になる', async () => {
    responses.ListInbox = { items: base, failures: [rateLimited] }
    const s = useInboxStore()
    await s.load()
    expect(s.failures).toHaveLength(1)
    responses.ListInbox = page(base)
    await s.load()
    expect(s.failures).toEqual([])
  })

  test('続きの読み込みで別のアカウントが失敗しても、先に出した失敗は消えない（読み直しで置き換わる）', async () => {
    const failed = (accountId: string) => ({
      accountId,
      error: { code: 'RATE_LIMITED', message: '制限', retryable: true },
    })
    responses.ListInbox = { items: base.slice(0, 2), nextCursor: 'n', failures: [failed('a1')] }
    const s = useInboxStore()
    await s.load()
    responses.ListInbox = { items: base.slice(2), failures: [failed('a2')] }
    await s.loadMore()
    expect(s.failures.map((f) => f.accountId).sort()).toEqual(['a1', 'a2'])
    responses.ListInbox = page(base)
    await s.load()
    expect(s.failures).toEqual([])
  })

  test('条件が変わって読み込めなかったとき、前の条件の failures を残さない', async () => {
    responses.ListInbox = { items: base, failures: [rateLimited] }
    const s = useInboxStore()
    await s.load()
    failures.Search = appError('INTERNAL')
    await s.runSearch('x')
    expect(s.items).toEqual([])
    expect(s.failures).toEqual([])
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

describe('検索の失敗ページ', () => {
  test('全アカウントが OFFLINE で失敗したら Search は先頭 1 回で止まり、自動で続きを投げ足さない', async () => {
    responses.Search = {
      items: [],
      nextCursor: 'c',
      failures: [
        { accountId: 'a1', error: { code: 'OFFLINE', message: 'x', retryable: true } },
        { accountId: 'a2', error: { code: 'OFFLINE', message: 'x', retryable: true } },
      ],
    }
    const s = useInboxStore()
    await s.runSearch('hit')
    expect(callsOf('Search')).toHaveLength(1)
    expect(s.items).toHaveLength(0)
    expect(s.nextCursor).toBe('c')
    await s.loadMore() // 利用者の再要求では再試行する
    expect(callsOf('Search')).toHaveLength(2)
  })
})

describe('読み込みの失敗と一覧', () => {
  test('検索の読み込みが失敗したら、前の受信箱の行と続きのカーソルを残さず、再試行で取り直せる', async () => {
    responses.ListInbox = { items: base, nextCursor: 'inbox-next', failures: [] }
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    expect(s.items).toHaveLength(4)
    expect(s.hasMore).toBe(true)

    failures.Search = appError('INTERNAL')
    await s.runSearch('x')
    expect(s.items).toEqual([])
    expect(s.hasMore).toBe(false)
    expect(ui.alerts).toHaveLength(1)

    delete failures.Search
    responses.Search = { items: [base[0]], failures: [] }
    ui.alerts[0].retry?.()
    await until(() => s.items.length === 1)
    expect(callsOf('Search')).toHaveLength(2)
  })

  test('OFFLINE の失敗でも、条件が変わっていれば前の行を残さない（通知は共通のオフライン表示に任せる）', async () => {
    const s = useInboxStore()
    await s.load()
    failures.Search = appError('OFFLINE')
    await s.runSearch('x')
    expect(s.items).toEqual([])
    expect(useUiStore().alerts).toHaveLength(0)
  })

  test('絞り込み（未読のみ）の読み込みが失敗したら、前の条件の行を残さない', async () => {
    const s = useInboxStore()
    await s.load()
    expect(s.items).toHaveLength(4)
    failures.ListInbox = appError('INTERNAL')
    await s.setFilter({ accountIds: [], unreadOnly: true })
    expect(s.items).toEqual([])
  })

  test('同じ条件の再読み込み（手動更新）が失敗しても、取得済みの行は保つ', async () => {
    const s = useInboxStore()
    await s.load()
    failures.ListInbox = appError('INTERNAL')
    await s.refresh()
    expect(s.items).toHaveLength(4)
  })
})
