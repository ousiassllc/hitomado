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
const { viewSearchQuery, unreadTotalOf, hidesReadOf } = await import('../../src/lib/inboxViews')

beforeEach(resetKit)

const inInbox = (t: ReturnType<typeof thread>, v: boolean) => ({ ...t, inInbox: v })

describe('見る場所（受信箱・下書き・送信済み・アーカイブ）', () => {
  test('送信済みは Gmail の検索で引き、アカウントの絞り込みと組み合わせて効く（仕事アカウントの送信済みだけ）', async () => {
    responses.Search = page([inInbox(thread('a1', 's1', 100, false), false)])
    const s = useInboxStore()
    await s.load()
    await s.setFilter({ accountIds: ['a1'], unreadOnly: false })
    await s.setView('sent')

    const last = callsOf('Search').at(-1)?.args[0] as { q: string; accountIds: string[] }
    expect(last.q).toBe('in:sent')
    expect(last.accountIds).toEqual(['a1'])
    expect(ids(s)).toEqual(['s1'])
    expect(s.view).toBe('sent')
  })

  test('アーカイブは受信箱にもあるスレッドを落とす（Gmail は 1 通でも合えばスレッドを返すため）', async () => {
    responses.Search = page([
      inInbox(thread('a1', 'old', 100, false), false),
      inInbox(thread('a1', 'also-in-inbox', 90, false), true),
    ])
    const s = useInboxStore()
    await s.setView('archive')
    const last = callsOf('Search').at(-1)?.args[0] as { q: string } | undefined
    expect(last?.q).toBe('-in:inbox')
    expect(ids(s)).toEqual(['old'])
  })

  test('下書きは一覧ページ（#366）が入るまで準備中で、何も引かない', async () => {
    const s = useInboxStore()
    await s.load()
    const before = [callsOf('Search').length, callsOf('ListInbox').length]
    await s.setView('drafts')
    expect(s.view).toBe('drafts')
    expect(ids(s)).toEqual([])
    expect([callsOf('Search').length, callsOf('ListInbox').length]).toEqual(before)
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

  test('未読のみはアーカイブでは is:unread で効かせ、送信済み・下書きでは効かせない（掛けると必ず空になる）', () => {
    expect(viewSearchQuery('archive', true)).toBe('-in:inbox is:unread')
    expect(viewSearchQuery('archive', false)).toBe('-in:inbox')
    expect(viewSearchQuery('sent', true)).toBe('in:sent')
    expect(viewSearchQuery('inbox', true)).toBeUndefined()
    expect(viewSearchQuery('drafts', true)).toBeUndefined()
    expect(hidesReadOf('inbox', 'sent', 'all', true)).toBe(false)
    expect(hidesReadOf('inbox', 'archive', 'all', true)).toBe(true)
    expect(hidesReadOf('inbox', 'inbox', 'new', false)).toBe(true)
    expect(hidesReadOf('search', 'inbox', 'new', true)).toBe(false)
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
    await s.setView('sent')
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
