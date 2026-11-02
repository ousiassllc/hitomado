import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  callsOf,
  deferred,
  failures,
  gates,
  gone,
  ids,
  installMocks,
  markGone,
  ref,
  resetKit,
  responses,
  restoreMocks,
  until,
} from './inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../src/stores/inbox')
const { useUiStore } = await import('../src/stores/ui')

beforeEach(resetKit)

describe('楽観的更新', () => {
  test('archive は呼び出し完了前にリストから消え、成功後も復活しない', async () => {
    const g = deferred()
    gates.Archive = g.p
    const s = useInboxStore()
    await s.load()
    const done = s.archive([ref('a1', 't1')])
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    g.resolve()
    expect(await done).toBe(true)
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    expect(callsOf('Archive')[0].args[0]).toEqual([ref('a1', 't1')])
  })

  test('失敗時は元の位置へ復元し、再試行付きエラートーストを出す', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    expect(await s.archive([ref('a2', 't3')])).toBe(false)
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
    expect(ui.toasts).toHaveLength(1)
    expect(ui.toasts[0].kind).toBe('error')
    expect(typeof ui.toasts[0].retry).toBe('function')

    delete failures.Archive
    ui.toasts[0].retry?.()
    await until(() => callsOf('Archive').length === 2)
    expect(ids(s)).toEqual(['t1', 't2', 't4'])
  })

  test('複数選択は選択全体に作用する', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    s.toggleChecked(ref('a1', 't1'))
    s.toggleChecked(ref('a2', 't3'))
    expect(s.targetRefs).toEqual([ref('a1', 't1'), ref('a2', 't3')])
    await s.trash(s.targetRefs)
    expect(callsOf('Trash')[0].args[0]).toEqual([ref('a1', 't1'), ref('a2', 't3')])
    expect(ids(s)).toEqual(['t2', 't4'])
    expect(s.checkedKeys).toEqual([])
    expect(s.selectedKey).toBeUndefined()
  })

  test('一部の ref だけ失敗したら失敗分だけ復元する', async () => {
    failures.Trash = appError('GMAIL_ERROR', { details: { failedRefs: [ref('a2', 't3')] } })
    const s = useInboxStore()
    await s.load()
    gone.add('t1') // t1 はサーバー側で成功済み
    await s.trash([ref('a1', 't1'), ref('a2', 't3')])
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
  })

  test('NOT_FOUND は failedRefs の行を復元せず一覧から除去し、「見つかりません」トーストを出す', async () => {
    failures.Archive = appError('NOT_FOUND', { details: { failedRefs: [ref('a2', 't3')] } })
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    const hold = deferred()
    gates.ListInbox = hold.p // 操作後の再読込を保留し、操作直後の一覧を検査する
    gone.add('t1') // t1 はサーバー側でアーカイブ成功済み（failedRefs に含まれない）
    gone.add('t3') // t3 はサーバー側に存在しない
    expect(await s.archive([ref('a1', 't1'), ref('a2', 't3')])).toBe(false)
    // 再読込の前でも、成功した t1 は一覧へ戻らない（復元しない）
    expect(ids(s)).toEqual(['t2', 't4'])
    expect(ids(s)).toContain('t2')
    expect(ui.toasts).toHaveLength(1)
    expect(ui.toasts[0].message).toContain('見つかりません')
    expect(ui.toasts[0].retry).toBeUndefined()
    hold.resolve()
  })

  test('NOT_FOUND 以外の失敗は従来どおり復元＋再試行トースト', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    await s.archive([ref('a2', 't3')])
    expect(ids(s)).toContain('t3')
    expect(typeof ui.toasts[0].retry).toBe('function')
  })

  test('markRead は即時に既読表示になり、失敗時は未読に戻る', async () => {
    failures.MarkRead = appError('GMAIL_ERROR')
    const g = deferred()
    gates.MarkRead = g.p
    const s = useInboxStore()
    await s.load()
    const done = s.markRead([ref('a1', 't1')], true)
    expect(s.items[0].isUnread).toBe(false)
    g.resolve()
    expect(await done).toBe(false)
    expect(s.items[0].isUnread).toBe(true)
  })

  test('未読のみフィルタ中の既読化はリストから除去し、失敗で戻す', async () => {
    failures.MarkRead = appError('GMAIL_ERROR')
    const s = useInboxStore()
    await s.setFilter({ accountIds: [], unreadOnly: true })
    const done = s.markRead([ref('a1', 't2')], true)
    expect(ids(s)).toEqual(['t1', 't3', 't4'])
    await done
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
  })

  test('未読のみ表示中の既読化では開いているスレッドの選択を維持する', async () => {
    responses.MarkRead = markGone
    const s = useInboxStore()
    await s.setFilter({ accountIds: [], unreadOnly: true })
    s.select(ref('a1', 't2'))
    await s.markRead([ref('a1', 't2')], true)
    expect(ids(s)).not.toContain('t2')
    expect(s.selectedKey).toBe('a1:t2')
  })

  test('除去した行の次の行へカーソルが移る', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    await s.archive([ref('a1', 't2')])
    expect(s.cursorKey).toBe('a2:t3')
    expect(s.selectedKey).toBeUndefined()
  })

  test('処理中に先頭ページを再取得しても除去済みの行を復活させない', async () => {
    const g = deferred()
    gates.Archive = g.p
    const s = useInboxStore()
    await s.load()
    const done = s.archive([ref('a1', 't1')])
    s.onSyncUpdated({ accountId: 'a1', changedThreadIds: [] })
    await until(() => callsOf('GetUnreadCounts').length >= 2)
    expect(callsOf('ListInbox')).toHaveLength(2)
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    g.resolve()
    await done
  })

  test('removeThread はサーバーを呼ばずに除去し、選択も外す', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    s.removeThread(ref('a1', 't1'))
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    expect(s.selectedRef).toBeUndefined()
    expect(callsOf('Archive')).toHaveLength(0)
  })
})
