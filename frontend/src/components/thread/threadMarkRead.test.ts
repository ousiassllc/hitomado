import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import {
  appError,
  calls,
  callsOf,
  ctl,
  detail,
  installMocks,
  resetState,
  restoreMocks,
  until,
} from './threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../stores/thread')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})

const A = { accountId: 'a1', threadId: 'tA' }
const B = { accountId: 'a1', threadId: 'tB' }

function threadOf(ref: { threadId: string }, unread: boolean) {
  return detail(unread, { threadId: ref.threadId, subject: `件名-${ref.threadId}` })
}

interface Deferred<T> {
  promise: Promise<T>
  resolve: (v: T) => void
}

function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

/** GetThread を ref ごとのレスポンス（Promise 可）で返す。 */
function routeGetThread(byThread: Record<string, unknown>): void {
  ctl.responses.GetThread = (ref: { threadId: string }) => byThread[ref.threadId]
}

describe('threadStore: 既読化はスレッド単位', () => {
  test('A の既読化中に B を開いても、B は自動既読になる', async () => {
    const markA = deferred<unknown[]>()
    routeGetThread({ tA: threadOf(A, true), tB: threadOf(B, true) })
    ctl.responses.MarkRead = (refs: { threadId: string }[]) =>
      refs[0].threadId === 'tA' ? markA.promise : []
    const s = useThreadStore()

    await s.open(A)
    await until(() => callsOf('MarkRead').length === 1) // A の既読化が保留中
    await s.open(B)
    await until(() => callsOf('MarkRead').length === 2)
    expect(callsOf('MarkRead')[1].args).toEqual([[B], true])
    await until(() => !s.unread)
    expect(s.currentRef).toEqual(B)

    // 保留中だった A が後から確定しても、B の表示状態は変わらない
    markA.resolve([])
    await Promise.resolve()
    expect(s.currentRef).toEqual(B)
    expect(s.current?.threadId).toBe('tB')
  })

  test('A→B→A と開き直すと、既読化済みの A では再度呼ばない', async () => {
    routeGetThread({ tA: threadOf(A, true), tB: threadOf(B, false) })
    const s = useThreadStore()
    await s.open(A)
    await until(() => !s.unread)
    expect(callsOf('MarkRead')).toHaveLength(1)
    await s.open(B)
    routeGetThread({ tA: threadOf(A, false), tB: threadOf(B, false) })
    await s.open(A)
    expect(callsOf('MarkRead')).toHaveLength(1)
  })

  test('既読化に失敗しても未読メッセージは見たことにならず、再取得で再試行される', async () => {
    routeGetThread({ tA: threadOf(A, true) })
    ctl.failures.MarkRead = appError('GMAIL_ERROR', '失敗')
    const s = useThreadStore()
    await s.open(A)
    await until(() => callsOf('MarkRead').length === 1)
    await until(() => callsOf('ListInbox').length >= 1) // inboxStore の失敗処理（復元・再読込）が済む
    expect(s.unread).toBe(true)

    delete ctl.failures.MarkRead
    await s.reload()
    await until(() => callsOf('MarkRead').length === 2)
    await until(() => !s.unread)
  })

  test('既読化中の再取得で、同じスレッドの既読化を二重に呼ばない', async () => {
    const mark = deferred<unknown[]>()
    routeGetThread({ tA: threadOf(A, true) })
    ctl.responses.MarkRead = () => mark.promise
    const s = useThreadStore()
    await s.open(A)
    await until(() => callsOf('MarkRead').length === 1)
    await s.reload()
    await s.reload()
    expect(callsOf('MarkRead')).toHaveLength(1)
    mark.resolve([])
    await until(() => !s.unread)
  })

  test('別スレッドへ移った後に確定した A の既読化は B の seenIds・unread を汚さない', async () => {
    const markA = deferred<unknown[]>()
    routeGetThread({ tA: threadOf(A, true), tB: threadOf(B, false) })
    ctl.responses.MarkRead = (refs: { threadId: string }[]) =>
      refs[0].threadId === 'tA' ? markA.promise : []
    const s = useThreadStore()
    await s.open(A)
    await until(() => callsOf('MarkRead').length === 1)
    await s.open(B)
    expect(s.unread).toBe(false)
    markA.resolve([])
    await Promise.resolve()
    await Promise.resolve()
    expect(s.unread).toBe(false)
    // B に新着の未読メッセージが増えたら、通常どおり自動既読になる
    const base = threadOf(B, false)
    const m1 = base.messages?.[0]
    if (!m1) throw new Error('fixture')
    routeGetThread({ tB: { ...base, messages: [m1, { ...m1, id: 'm2', isUnread: true }] } })
    await s.reload()
    await until(() =>
      callsOf('MarkRead').some((c) => (c.args[0] as { threadId: string }[])[0].threadId === 'tB'),
    )
  })
})

describe('threadStore: 古いレスポンス', () => {
  test('A の GetThread が遅れて返っても、表示中の B を上書きしない', async () => {
    const slowA = deferred<unknown>()
    routeGetThread({ tA: slowA.promise, tB: threadOf(B, false) })
    const s = useThreadStore()
    const openA = s.open(A)
    await s.open(B)
    expect(s.current?.threadId).toBe('tB')
    slowA.resolve(threadOf(A, true))
    await openA
    expect(s.current?.threadId).toBe('tB')
    expect(s.currentRef).toEqual(B)
    expect(s.loading).toBe(false)
    // 上書きされなかったので A の既読化も走らない
    expect(callsOf('MarkRead')).toHaveLength(0)
  })

  test('A の GetThread が遅れて NOT_FOUND を返しても B の表示を閉じない', async () => {
    let failA!: (e: unknown) => void
    const slowA = new Promise((_, rej) => {
      failA = rej
    })
    routeGetThread({ tA: slowA, tB: threadOf(B, false) })
    const s = useThreadStore()
    const openA = s.open(A)
    await s.open(B)
    failA(appError('NOT_FOUND'))
    await openA
    expect(s.current?.threadId).toBe('tB')
    expect(calls.some((c) => c.name === 'GetThread')).toBe(true)
  })
})
