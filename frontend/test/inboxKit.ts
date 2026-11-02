import { expect, mock } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realAccount from '../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import * as realApp from '../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import type { ThreadSummary } from '../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as realInbox from '../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import * as realEvents from '../src/lib/events'

const B = '../bindings/github.com/ousiassllc/hitomado/internal/app'

type Handler = (payload: unknown) => void
export const handlers = new Map<string, Handler>()
export const calls: { name: string; args: unknown[] }[] = []
/** 名前 → 次の呼び出しの応答（関数なら呼び出しごとに評価）。 */
export const responses: Record<string, unknown> = {}
export const failures: Record<string, unknown> = {}
export const gates: Record<string, Promise<void>> = {}
/** サーバー側で消えたスレッド（Archive/Trash 成功後の ListInbox に出さない）。 */
export const gone = new Set<string>()

function binding(name: string) {
  return async (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in gates) await gates[name]
    if (name in failures) throw failures[name]
    const r = responses[name]
    return typeof r === 'function' ? (r as (...a: unknown[]) => unknown)(...args) : r
  }
}

/** バインディング・イベントをモックする。ストアを import する前に呼ぶ。 */
export function installMocks(): void {
  mock.module(`${B}/accountservice`, () => ({
    ListAccounts: binding('ListAccounts'),
    ListOAuthClients: binding('ListOAuthClients'),
    CancelAddAccount: binding('CancelAddAccount'),
  }))
  mock.module(`${B}/appservice`, () => ({
    GetInboxFilter: binding('GetInboxFilter'),
    SetInboxFilter: binding('SetInboxFilter'),
  }))
  mock.module(`${B}/inboxservice`, () => ({
    ListInbox: binding('ListInbox'),
    Search: binding('Search'),
    GetUnreadCounts: binding('GetUnreadCounts'),
    Archive: binding('Archive'),
    Trash: binding('Trash'),
    MarkRead: binding('MarkRead'),
    Refresh: binding('Refresh'),
  }))
  mock.module('../src/lib/events', () => ({
    ...realEvents,
    onEvent: (name: string, h: Handler) => {
      handlers.set(name, h)
      return () => handlers.delete(name)
    },
  }))
}

/** 他のテストファイルへモックが漏れないよう、本物へ戻す。 */
export function restoreMocks(): void {

  mock.module(`${B}/accountservice`, () => ({ ...realAccount }))
  mock.module(`${B}/appservice`, () => ({ ...realApp }))
  mock.module(`${B}/inboxservice`, () => ({ ...realInbox }))
  mock.module('../src/lib/events', () => ({ ...realEvents }))
}


export function thread(account: string, id: string, at: number, unread = true): ThreadSummary {
  return {
    accountId: account,
    threadId: id,
    subject: `s-${id}`,
    snippet: '',
    lastMessageAt: at,
    lastFrom: { email: 'a@example.com' },
    messageCount: 1,
    isUnread: unread,
    hasAttachment: false,
    inInbox: true,
    labels: [],
  } as ThreadSummary
}

export const page = (items: ThreadSummary[], nextCursor?: string) => ({ items, nextCursor, failures: [] })
export const ref = (a: string, t: string) => ({ accountId: a, threadId: t })
export const callsOf = (name: string) => calls.filter((c) => c.name === name)
export const ids = (s: { items: ThreadSummary[] }) => s.items.map((t) => t.threadId)

export function appError(code: string, extra: Record<string, unknown> = {}) {
  return { cause: JSON.stringify({ code, message: `err ${code}`, retryable: true, ...extra }) }
}

export function deferred() {
  const { promise, resolve } = Promise.withResolvers<void>()
  return { p: promise, resolve }
}

/** 条件が満たされるまでマイクロタスクを回す（上限付き。固定時間は待たない）。 */
export async function until(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 2000 && !cond(); i++) await Promise.resolve()
  expect(cond()).toBe(true)
}

export const base: ThreadSummary[] = [thread('a1', 't1', 500), thread('a1', 't2', 400), thread('a2', 't3', 300), thread('a2', 't4', 200)]

export function markGone(refs: unknown) {
  for (const r of refs as { threadId: string }[]) gone.add(r.threadId)
  return []
}

/** テストごとの初期状態（Pinia・呼び出し記録・既定の応答）。 */
export function resetKit(): void {
  gone.clear()
  setActivePinia(createPinia())
  handlers.clear()
  calls.length = 0
  for (const o of [failures, gates, responses]) for (const key of Object.keys(o)) delete o[key]
  Object.assign(responses, {
    ListAccounts: [],
    ListOAuthClients: [],
    GetInboxFilter: { accountIds: null, unreadOnly: false },
    SetInboxFilter: undefined,
    ListInbox: () => page(base.filter((t) => !gone.has(t.threadId))),
    Search: page([]),
    GetUnreadCounts: { a1: 2, a2: 2 },
    Archive: markGone,
    Trash: markGone,
    MarkRead: [],
    Refresh: { results: [] },
  })
}