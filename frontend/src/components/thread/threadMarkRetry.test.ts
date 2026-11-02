import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import {
  appError,
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

type Msg = NonNullable<ReturnType<typeof detail>['messages']>[number]

function thread(ref: { threadId: string }, msgs: { id: string; unread: boolean }[]) {
  const base = detail(false).messages?.[0] as Msg
  return detail(false, {
    threadId: ref.threadId,
    messages: msgs.map((m) => ({ ...base, id: m.id, isUnread: m.unread })),
  })
}

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function markCalls() {
  return callsOf('MarkRead').map((c) => (c.args[0] as { threadId: string }[])[0].threadId)
}

describe('threadStore: 既読化の再評価と再試行の抑止', () => {
  test('(a) 既読化中に A→B→A と開き直して最初の既読化が失敗しても、UI は未読のままで一覧と食い違わない', async () => {
    const first = deferred<unknown[]>()
    let n = 0
    ctl.responses.GetThread = (ref: { threadId: string }) =>
      ref.threadId === 'tA'
        ? thread(A, [{ id: 'a1', unread: true }])
        : thread(B, [{ id: 'b1', unread: false }])
    ctl.responses.MarkRead = () => (n++ === 0 ? first.promise : [])
    const s = useThreadStore()

    await s.open(A)
    await until(() => markCalls().length === 1)
    await s.open(B)
    await s.open(A) // 最初の既読化が実行中のまま A に戻る（二重には呼ばない）
    expect(markCalls()).toHaveLength(1)

    first.reject(appError('GMAIL_ERROR', '失敗'))
    await until(() => s.unread)
    expect(s.unread).toBe(true)
    // 失敗した ID は自動再試行されない
    await s.reload()
    expect(markCalls()).toHaveLength(1)
    expect(s.unread).toBe(true)
  })

  test('(a) 既読化中に A→B→A と開き直して最初の既読化が成功したら、UI は既読になる', async () => {
    const first = deferred<unknown[]>()
    let n = 0
    ctl.responses.GetThread = (ref: { threadId: string }) =>
      ref.threadId === 'tA' ? thread(A, [{ id: 'a1', unread: true }]) : thread(B, [])
    ctl.responses.MarkRead = () => (n++ === 0 ? first.promise : [])
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    await s.open(B)
    await s.open(A)
    first.resolve([])
    await until(() => !s.unread)
    expect(markCalls()).toHaveLength(1)
  })

  test('(b) 既読化中に届いた新着の未読は、確定後に既読化される', async () => {
    const first = deferred<unknown[]>()
    let n = 0
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.responses.MarkRead = () => (n++ === 0 ? first.promise : [])
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)

    // 実行中に新着（m2）を含む再取得が来る。busy なのでこの時点では二重に呼ばない。
    ctl.responses.GetThread = thread(A, [
      { id: 'm1', unread: true },
      { id: 'm2', unread: true },
    ])
    await s.reload()
    expect(markCalls()).toHaveLength(1)

    first.resolve([])
    await until(() => markCalls().length === 2) // m2 のための既読化
    await until(() => !s.unread)
    expect(s.unread).toBe(false)
  })

  test('(b) 既読化中の新着が確定時に未処理のまま既読表示にならない（失敗側でも unread は真）', async () => {
    const first = deferred<unknown[]>()
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.responses.MarkRead = () => first.promise
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    ctl.responses.GetThread = thread(A, [
      { id: 'm1', unread: true },
      { id: 'm2', unread: true },
    ])
    await s.reload()
    first.reject(appError('GMAIL_ERROR', '失敗'))
    await until(() => s.unread)
    expect(s.unread).toBe(true)
  })

  test('(c) 失敗し続けても sync:updated のたびに再試行しない', async () => {
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.failures.MarkRead = appError('GMAIL_ERROR', '失敗')
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    await until(() => s.unread)
    for (let i = 0; i < 3; i++) await s.reload()
    expect(markCalls()).toHaveLength(1)
    expect(s.unread).toBe(true)
  })

  test('(c) 失敗後に新しい未読メッセージが来たら（それは別の ID なので）あらためて試みる', async () => {
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.failures.MarkRead = appError('GMAIL_ERROR', '失敗')
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    await until(() => s.unread)
    ctl.responses.GetThread = thread(A, [
      { id: 'm1', unread: true },
      { id: 'm2', unread: true },
    ])
    await s.reload()
    await until(() => markCalls().length === 2)
  })

  test('手動の既読／未読操作で失敗 ID の抑止が解除され、次の再取得で再試行される', async () => {
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.failures.MarkRead = appError('GMAIL_ERROR', '失敗')
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    await until(() => s.unread)
    await s.reload()
    expect(markCalls()).toHaveLength(1)

    await s.toggleRead() // 手動操作（失敗する）
    expect(markCalls()).toHaveLength(2)
    await s.reload()
    await until(() => markCalls().length === 3)
  })

  test('既読化が重なっても marking はカウンタで管理され、手動操作の完了で自動既読の実行中表示が消えない', async () => {
    const auto = deferred<unknown[]>()
    let n = 0
    ctl.responses.GetThread = thread(A, [{ id: 'm1', unread: true }])
    ctl.responses.MarkRead = () => (n++ === 0 ? auto.promise : [])
    const s = useThreadStore()
    await s.open(A)
    await until(() => markCalls().length === 1)
    await s.toggleRead() // 手動操作が先に完了する
    // 自動既読はまだ実行中。再取得しても二重に呼ばない
    await s.reload()
    expect(markCalls()).toHaveLength(2)
    auto.resolve([])
    await Promise.resolve()
  })
})
