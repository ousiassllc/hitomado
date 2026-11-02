import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { callsOf, handlers, ids, installMocks, page, ref, resetKit, responses, restoreMocks, thread, until } from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../src/stores/inbox')
const { useUiStore } = await import('../src/stores/ui')
const { useAccountsStore } = await import('../src/stores/accounts')

beforeEach(resetKit)

describe('start / フィルタ', () => {
  test('保存済みフィルタを復元してから先頭ページと未読件数を取得する', async () => {
    responses.GetInboxFilter = { accountIds: ['a2'], unreadOnly: true }
    const s = useInboxStore()
    await s.start()
    expect(s.filter).toEqual({ accountIds: ['a2'], unreadOnly: true })
    expect(callsOf('ListInbox')[0].args[0]).toMatchObject({ accountIds: ['a2'], unreadOnly: true, limit: 50 })
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
    expect(s.unreadCounts).toEqual({ a1: 2, a2: 2 })
    expect(handlers.has('sync:updated')).toBe(true)
  })

  test('start は冪等', async () => {
    const s = useInboxStore()
    await s.start()
    await s.start()
    expect(callsOf('GetInboxFilter')).toHaveLength(1)
  })

  test('setFilter は保存して再取得する', async () => {
    const s = useInboxStore()
    await s.setFilter({ accountIds: ['a1'], unreadOnly: true })
    expect(callsOf('SetInboxFilter')[0].args[0]).toEqual({ accountIds: ['a1'], unreadOnly: true })
    expect(callsOf('ListInbox')[0].args[0]).toMatchObject({ accountIds: ['a1'], unreadOnly: true })
  })
})

describe('sync:updated', () => {
  test('先頭ページを差し替え、選択と読み込み済みの続きを維持する', async () => {
    let n = 0
    responses.ListInbox = (q: unknown) => {
      if ((q as { cursor?: string }).cursor) return page([thread('a2', 't5', 100)])
      n++
      return n === 1
        ? page([thread('a1', 't1', 500), thread('a1', 't2', 400)], 'c1')
        : page([thread('a1', 'tn', 600), thread('a1', 't1', 500), thread('a1', 't2', 400)], 'c1')
    }
    const s = useInboxStore()
    await s.start()
    await s.loadMore()
    expect(ids(s)).toEqual(['t1', 't2', 't5'])
    s.select(ref('a1', 't2'))
    handlers.get('sync:updated')?.({ accountId: 'a1', changedThreadIds: ['tn'] })
    await until(() => ids(s)[0] === 'tn')
    expect(ids(s)).toEqual(['tn', 't1', 't2', 't5'])
    expect(s.selectedKey).toBe('a1:t2')
    expect(callsOf('GetUnreadCounts').length).toBeGreaterThanOrEqual(2)
  })
})

describe('検索', () => {
  test('runSearch は検索モードにフィルタを引き継ぎ、clearSearch で受信箱に戻る', async () => {
    responses.Search = page([thread('a1', 'tx', 100)])
    const s = useInboxStore()
    await s.setFilter({ accountIds: ['a1'], unreadOnly: false })
    await s.runSearch(' from:x ')
    expect(s.mode).toBe('search')
    expect(callsOf('Search')[0].args[0]).toMatchObject({ q: 'from:x', accountIds: ['a1'], limit: 50 })
    expect(ids(s)).toEqual(['tx'])
    // 検索中の操作もそのまま使える
    await s.archive([ref('a1', 'tx')])
    expect(ids(s)).toEqual([])
    // sync:updated は検索結果を差し替えない
    const before = callsOf('Search').length
    s.onSyncUpdated({ accountId: 'a1', changedThreadIds: [] })
    await until(() => callsOf('GetUnreadCounts').length >= 2)
    expect(callsOf('Search').length).toBe(before)

    await s.clearSearch()
    expect(s.mode).toBe('inbox')
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
  })

  test('空のクエリは検索をクリアする', async () => {
    const s = useInboxStore()
    await s.runSearch('x')
    await s.runSearch('  ')
    expect(s.mode).toBe('inbox')
  })
})

