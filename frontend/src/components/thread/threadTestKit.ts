// threadStore のテスト用ハーネス（バインディング・イベントのモックと共通のフィクスチャ）。
import { expect, mock } from 'bun:test'
import * as realAccount from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import * as realApp from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import type { ThreadDetail } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as realInbox from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import * as realNotify from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/notifycore/notifyservice'
import * as realThread from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/threadservice'
import * as realEvents from '../../lib/events'

const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'

type Handler = (payload: unknown) => void

// 他のテストファイルの lib/events モックが本物の import を汚すことがあるため、イベント名は固定で持つ。
const EVENTS = {
  SYNC_UPDATED: 'sync:updated',
  ACCOUNT_STATUS: 'account:status',
  BEFORE_QUIT: 'app:before-quit',
  NOTIFY_OPEN_THREAD: 'notify:open-thread',
  UPDATE_AVAILABLE: 'update:available',
} as const

export const handlers = new Map<string, Handler>()
export const calls: { name: string; args: unknown[] }[] = []
export const ctl: { responses: Record<string, unknown>; failures: Record<string, unknown> } = {
  responses: {},
  failures: {},
}

function binding(name: string) {
  return async (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in ctl.failures) throw ctl.failures[name]
    const r = ctl.responses[name]
    return typeof r === 'function' ? (r as (...a: unknown[]) => unknown)(...args) : r
  }
}

/** 使うバインディングとイベント購読をモックする。ストアを import する前に呼ぶ。 */
export function installMocks(): void {
  mock.module(`${B}/threadservice`, () => ({
    GetThread: binding('GetThread'),
    SaveAttachment: binding('SaveAttachment'),
    OpenExternal: binding('OpenExternal'),
  }))
  mock.module(`${B}/inboxservice`, () => ({
    ListInbox: binding('ListInbox'),
    GetUnreadCounts: binding('GetUnreadCounts'),
    MarkRead: binding('MarkRead'),
    Archive: binding('Archive'),
    Trash: binding('Trash'),
    ListLabels: binding('ListLabels'),
    ModifyLabels: binding('ModifyLabels'),
  }))
  mock.module(`${B}/appservice`, () => ({
    GetInboxFilter: binding('GetInboxFilter'),
    SetInboxFilter: binding('SetInboxFilter'),
  }))
  mock.module(`${B}/accountservice`, () => ({}))
  mock.module(`${B}/notifycore/notifyservice`, () => ({
    TakePendingOpenThread: binding('TakePendingOpenThread'),
  }))
  mock.module('../../lib/events', () => ({
    EVENTS,
    onEvent: (name: string, h: Handler) => {
      handlers.set(name, h)
      return () => handlers.delete(name)
    },
  }))
}

/** 他のテストファイルへモックが漏れないよう、本物へ戻す。 */
export function restoreMocks(): void {
  mock.module(`${B}/threadservice`, () => ({ ...realThread }))
  mock.module(`${B}/inboxservice`, () => ({ ...realInbox }))
  mock.module(`${B}/appservice`, () => ({ ...realApp }))
  mock.module(`${B}/accountservice`, () => ({ ...realAccount }))
  mock.module(`${B}/notifycore/notifyservice`, () => ({ ...realNotify }))
  mock.module('../../lib/events', () => ({ ...realEvents, EVENTS }))
}

export const REF = { accountId: 'a1', threadId: 't1' }

export function detail(unread: boolean, extra: Partial<ThreadDetail> = {}): ThreadDetail {
  return {
    account: {
      id: 'a1',
      oauthClientId: 'c1',
      email: 'me@example.com',
      displayName: '個人',
      color: 'blue',
      signatureHtml: '',
      notifyEnabled: true,
      status: 'active',
      sortOrder: 0,
    },
    threadId: 't1',
    subject: '件名',
    messages: [
      {
        id: 'm1',
        from: { email: 'x@example.com' },
        to: null,
        cc: null,
        date: 1,
        isUnread: unread,
        bodyText: 'hi',
        hasRemoteImages: false,
        attachments: null,
      },
    ],
    labels: null,
    ...extra,
  }
}

export function callsOf(name: string) {
  return calls.filter((c) => c.name === name)
}

/** 上限付きの条件待ち（固定時間の sleep に頼らない）。 */
export async function until(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 200 && !cond(); i++) await new Promise((r) => setTimeout(r, 0))
  expect(cond()).toBe(true)
}

export const appError = (code: string, message = 'エラー') => ({
  cause: JSON.stringify({ code, message, retryable: false }),
})

/** 各テストの前に呼ぶ。 */
export function resetState(): void {
  handlers.clear()
  calls.length = 0
  ctl.failures = {}
  ctl.responses = {
    GetThread: detail(true),
    TakePendingOpenThread: null,
    MarkRead: [],
    ListInbox: { items: [], failures: [] },
    GetUnreadCounts: {},
    GetInboxFilter: { accountIds: [], unreadOnly: false },
    ListLabels: [{ id: 'L1', name: '仕事' }],
    ModifyLabels: { accountId: 'a1', threadId: 't1', labels: [] },
  }
}
