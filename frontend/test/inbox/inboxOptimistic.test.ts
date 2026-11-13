import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  base,
  callsOf,
  deferred,
  failures,
  gates,
  gone,
  ids,
  installMocks,
  markGone,
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

beforeEach(resetKit)

describe('楽観的更新の失敗・既読化・除去', () => {
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
  test('一部の ref だけ失敗したら失敗分だけ復元する', async () => {
    failures.Trash = appError('GMAIL_ERROR', { details: { failedRefs: [ref('a2', 't3')] } })
    const s = useInboxStore()
    await s.load()
    gone.add('t1') // t1 はサーバー側で成功済み
    await s.trash([ref('a1', 't1'), ref('a2', 't3')])
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
  })

  test('NOT_FOUND は failedRefs の行を復元せず一覧から除去し、「見つかりません」アラートを出す', async () => {
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
    expect(ui.alerts).toHaveLength(1)
    expect(ui.alerts[0].message).toContain('見つかりません')
    expect(ui.alerts[0].retry).toBeUndefined()
    hold.resolve()
  })

  test('NOT_FOUND 以外の失敗は従来どおり復元＋再試行アラート', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    await s.archive([ref('a2', 't3')])
    expect(ids(s)).toContain('t3')
    expect(typeof ui.alerts[0].retry).toBe('function')
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

  test('失敗時は元の位置へ復元し、再試行付きエラーアラートを出す', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    expect(await s.archive([ref('a2', 't3')])).toBe(false)
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
    expect(ui.alerts).toHaveLength(1)
    expect(ui.alerts[0].kind).toBe('error')
    expect(typeof ui.alerts[0].retry).toBe('function')

    delete failures.Archive
    ui.alerts[0].retry?.()
    await until(() => callsOf('Archive').length === 2)
    expect(ids(s)).toEqual(['t1', 't2', 't4'])
  })

  test('最後の 1 件の失敗でも、空表示にしていたなら元のスレッドを開き直す', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    responses.ListInbox = () => page([base[0]])
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    expect(await s.archive([ref('a1', 't1')])).toBe(false)
    expect(ids(s)).toEqual(['t1'])
    expect(s.selectedKey).toBe('a1:t1')
    expect(s.cursorKey).toBe('a1:t1')
  })

  test('開いていないスレッドの失敗では、選択は変わらない', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    expect(await s.archive([ref('a1', 't2')])).toBe(false)
    expect(s.selectedKey).toBe('a1:t1')
    s.clearSelection()
    expect(await s.archive([ref('a1', 't2')])).toBe(false)
    expect(s.selectedKey).toBeUndefined()
  })

  test('最後の 1 件の失敗でも、その間にユーザーが開き直したスレッドは変えない', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const g = deferred()
    gates.Archive = g.p
    responses.ListInbox = () => page([base[0], base[1]])
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    const done = s.archive([ref('a1', 't1'), ref('a1', 't2')])
    s.select(ref('a1', 't2'))
    g.resolve()
    expect(await done).toBe(false)
    expect(s.selectedKey).toBe('a1:t2')
  })

  test('開いていない行を除去したときは、カーソルだけ次の行へ移り、開いているスレッドは変わらない', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    s.moveCursor(1)
    expect(s.cursorKey).toBe('a1:t2')
    await s.archive([ref('a1', 't2')])
    expect(s.cursorKey).toBe('a2:t3')
    expect(s.selectedKey).toBe('a1:t1')
  })
})
