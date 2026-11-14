import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  callsOf,
  ids,
  installMocks,
  page,
  resetKit,
  responses,
  restoreMocks,
  thread,
} from '../inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../../src/stores/inbox')
const { viewSearchQuery, unreadTotalOf } = await import('../../src/lib/inboxViews')

beforeEach(resetKit)

describe('見る場所（受信箱・下書き・送信済み・アーカイブ）', () => {
  test('受信箱以外は Gmail の検索で引き、アカウントの絞り込みと組み合わせて効く（仕事アカウントの下書きだけ）', async () => {
    responses.Search = page([thread('a1', 'd1', 100, false)])
    const s = useInboxStore()
    await s.load()
    await s.setFilter({ accountIds: ['a1'], unreadOnly: false })
    await s.setView('drafts')

    const last = callsOf('Search').at(-1)?.args[0] as { q: string; accountIds: string[] }
    expect(last.q).toBe('in:drafts')
    expect(last.accountIds).toEqual(['a1'])
    expect(ids(s)).toEqual(['d1'])
    expect(s.view).toBe('drafts')
  })

  test('受信箱へ戻すと ListInbox で引き直し、絞り込みと未読のみは保つ', async () => {
    const s = useInboxStore()
    await s.setFilter({ accountIds: ['a2'], unreadOnly: true })
    await s.setView('sent')
    const before = callsOf('ListInbox').length
    await s.setView('inbox')

    expect(callsOf('ListInbox')).toHaveLength(before + 1)
    const q = callsOf('ListInbox').at(-1)?.args[0] as { accountIds: string[]; unreadOnly: boolean }
    expect(q.accountIds).toEqual(['a2'])
    expect(q.unreadOnly).toBe(true)
  })

  test('未読のみは受信箱以外では is:unread で効かせる', () => {
    expect(viewSearchQuery('sent', true)).toBe('in:sent is:unread')
    expect(viewSearchQuery('archive', false)).toBe('-in:inbox -in:sent -in:drafts')
    expect(viewSearchQuery('inbox', true)).toBeUndefined()
  })

  test('検索中に場所を切り替えると検索をやめる。同じ場所を押し直しても取り直さない', async () => {
    const s = useInboxStore()
    await s.runSearch('見積')
    await s.setView('archive')
    expect(s.mode).toBe('inbox')
    expect(s.searchQuery).toBe('')
    const n = callsOf('Search').length
    await s.setView('archive')
    expect(callsOf('Search')).toHaveLength(n)
  })

  test('同期の更新では、受信箱以外の場所を取り直さない（同期のたびに Gmail の検索を呼ばない）', async () => {
    const s = useInboxStore()
    await s.setView('drafts')
    const n = callsOf('Search').length
    s.onSyncUpdated({ accountId: 'a1', changedThreadIds: [] })
    await Promise.resolve()
    expect(callsOf('Search')).toHaveLength(n)
  })

  test('受信箱の未読数は、絞り込み中は選んだアカウントの分だけ', () => {
    expect(unreadTotalOf({ a1: 3, a2: 4 }, [])).toBe(7)
    expect(unreadTotalOf({ a1: 3, a2: 4 }, ['a2'])).toBe(4)
  })
})
