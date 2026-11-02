import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  base,
  callsOf,
  deferred,
  gates,
  handlers,
  ids,
  installMocks,
  page,
  ref,
  resetKit,
  responses,
  restoreMocks,
  thread,
  until,
} from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../src/stores/inbox')

beforeEach(resetKit)

describe('start / フィルタ', () => {
  test('保存済みフィルタを復元してから先頭ページと未読件数を取得する', async () => {
    responses.GetInboxFilter = { accountIds: ['a2'], unreadOnly: true }
    const s = useInboxStore()
    await s.start()
    expect(s.filter).toEqual({ accountIds: ['a2'], unreadOnly: true })
    expect(callsOf('ListInbox')[0].args[0]).toMatchObject({
      accountIds: ['a2'],
      unreadOnly: true,
      limit: 50,
    })
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

describe('空ページ＋cursor', () => {
  test('先頭ページが 0 件でも続きがあれば、行が見つかるまで辿る', async () => {
    responses.ListInbox = (q: unknown) => {
      const c = (q as { cursor?: string }).cursor
      if (c === 'c2') return page([thread('a1', 't9', 100)])
      return page([], c ? 'c2' : 'c1')
    }
    const s = useInboxStore()
    await s.load()
    expect(ids(s)).toEqual(['t9'])
    expect(s.hasMore).toBe(false)
    expect(callsOf('ListInbox').map((c) => (c.args[0] as { cursor?: string }).cursor)).toEqual([
      undefined,
      'c1',
      'c2',
    ])
  })

  test('空ページが延々続いても上限で止まる', async () => {
    responses.ListInbox = () => page([], 'next')
    const s = useInboxStore()
    await s.load()
    expect(s.items).toEqual([])
    expect(callsOf('ListInbox').length).toBeLessThanOrEqual(12)
    expect(s.hasMore).toBe(true)
  })
})

describe('世代管理', () => {
  test('loadMore の応答前にフィルタ変更で再読込しても loadingMore が残らず、続きを読み込める', async () => {
    const g = deferred()
    responses.ListInbox = (q: unknown) =>
      (q as { cursor?: string }).cursor
        ? page([thread('a2', 'old', 100)])
        : page(base.slice(0, 2), 'c1')
    const s = useInboxStore()
    await s.load()
    gates.ListInbox = g.p
    const more = s.loadMore() // ゲートで待たされる
    expect(s.loadingMore).toBe(true)
    delete gates.ListInbox
    await s.setFilter({ accountIds: ['a1'], unreadOnly: false })
    expect(s.loadingMore).toBe(false)
    g.resolve()
    await more
    expect(s.loadingMore).toBe(false)
    expect(ids(s)).toEqual(['t1', 't2']) // 古い世代の続きは連結されない
    await s.loadMore()
    expect(ids(s)).toEqual(['t1', 't2', 'old'])
  })

  test('未読件数は新しい取得の結果を採用し、遅れて届いた古い応答で上書きしない', async () => {
    const first = deferred()
    let n = 0
    responses.GetUnreadCounts = async () => {
      n++
      if (n === 1) await first.p
      return n === 1 ? { a1: 99 } : { a1: 1 }
    }
    const s = useInboxStore()
    const stale = s.load() // 1 回目の件数取得は保留
    await until(() => n === 1)
    s.onSyncUpdated({ accountId: 'a1', changedThreadIds: [] }) // 2 回目
    await until(() => s.unreadCounts.a1 === 1)
    first.resolve()
    await stale
    expect(s.unreadCounts).toEqual({ a1: 1 })
  })
})

describe('検索', () => {
  test('runSearch は検索モードにフィルタを引き継ぎ、clearSearch で受信箱に戻る', async () => {
    responses.Search = page([thread('a1', 'tx', 100)])
    const s = useInboxStore()
    await s.setFilter({ accountIds: ['a1'], unreadOnly: false })
    await s.runSearch(' from:x ')
    expect(s.mode).toBe('search')
    expect(callsOf('Search')[0].args[0]).toMatchObject({
      q: 'from:x',
      accountIds: ['a1'],
      limit: 50,
    })
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
