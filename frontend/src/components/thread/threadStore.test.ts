import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import {
  appError,
  calls,
  callsOf,
  ctl,
  detail,
  handlers,
  installMocks,
  REF,
  resetState,
  restoreMocks,
  until,
} from './threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../stores/thread')
const { useInboxStore } = await import('../../stores/inbox')
const { useUiStore } = await import('../../stores/ui')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})

describe('threadStore', () => {
  test('open で GetThread を呼び、null のスライスを空配列に正規化する', async () => {
    const s = useThreadStore()
    await s.open(REF)
    expect(callsOf('GetThread')[0].args[0]).toEqual(REF)
    expect(s.current?.messages?.[0].to).toEqual([])
    expect(s.current?.messages?.[0].attachments).toEqual([])
    expect(s.current?.labels).toEqual([])
    expect(s.loading).toBe(false)
  })

  test('表示後に未読なら markRead し、既読なら呼ばない', async () => {
    const s = useThreadStore()
    await s.open(REF)
    await until(() => callsOf('MarkRead').length === 1)
    expect(callsOf('MarkRead')[0].args).toEqual([[REF], true])
    await until(() => !s.unread)

    calls.length = 0
    ctl.responses.GetThread = detail(false)
    s.close()
    await s.open(REF)
    await nextTick()
    expect(callsOf('MarkRead')).toHaveLength(0)
  })

  test('同じスレッドを開き直しても再取得しない（冪等）', async () => {
    const s = useThreadStore()
    await s.open(REF)
    await s.open(REF)
    expect(callsOf('GetThread')).toHaveLength(1)
  })

  test('NOT_FOUND はトーストを出して一覧から除去し、表示を閉じる', async () => {
    const inbox = useInboxStore()
    inbox.items = [
      {
        accountId: 'a1',
        threadId: 't1',
        subject: 's',
        snippet: '',
        lastMessageAt: 1,
        lastFrom: { email: 'x@example.com' },
        messageCount: 1,
        isUnread: false,
        hasAttachment: false,
        inInbox: true,
        labels: [],
      },
    ]
    inbox.select(REF)
    ctl.failures.GetThread = appError('NOT_FOUND')
    const s = useThreadStore()
    await s.open(REF)
    expect(useUiStore().toasts.map((t) => t.message)).toContain('スレッドが見つかりません')
    expect(inbox.items).toHaveLength(0)
    expect(inbox.selectedRef).toBeUndefined()
    expect(s.current).toBeUndefined()
    expect(s.currentRef).toBeUndefined()
  })

  test('その他のエラーは error に保持し、再取得（reload）で復帰できる', async () => {
    ctl.failures.GetThread = appError('GMAIL_ERROR', '失敗')
    const s = useThreadStore()
    await s.open(REF)
    expect(s.error?.code).toBe('GMAIL_ERROR')
    expect(s.current).toBeUndefined()
    ctl.failures = {}
    await s.reload()
    expect(s.error).toBeUndefined()
    expect(s.current?.threadId).toBe('t1')
  })

  test('再取得の失敗では表示中の内容を残す', async () => {
    const s = useThreadStore()
    await s.open(REF)
    ctl.failures.GetThread = appError('OFFLINE')
    await s.reload()
    expect(s.current?.threadId).toBe('t1')
    expect(s.error).toBeUndefined()
  })

  test('sync:updated は開いているスレッドが変更されたときだけ reload する', async () => {
    const s = useThreadStore()
    s.start()
    await s.open(REF)
    calls.length = 0
    handlers.get('sync:updated')?.({ accountId: 'a1', changedThreadIds: ['other'] })
    handlers.get('sync:updated')?.({ accountId: 'a2', changedThreadIds: ['t1'] })
    expect(callsOf('GetThread')).toHaveLength(0)
    handlers.get('sync:updated')?.({ accountId: 'a1', changedThreadIds: ['t1'] })
    await until(() => callsOf('GetThread').length === 1)
  })

  test('notify:open-thread で一覧を選択してスレッドを開く', async () => {
    const s = useThreadStore()
    s.start()
    handlers.get('notify:open-thread')?.({ accountId: 'a1', threadId: 't1' })
    await until(() => s.current !== undefined)
    expect(useInboxStore().selectedRef).toEqual(REF)
  })

  test('通知のタップで起動された（コールドスタート）ときは、購読の開始後に保留中のスレッドを開く', async () => {
    ctl.responses.TakePendingOpenThread = { accountId: 'a1', threadId: 't1' }
    const s = useThreadStore()
    s.start()
    await until(() => s.current !== undefined)
    expect(useInboxStore().selectedRef).toEqual(REF)
    expect(callsOf('TakePendingOpenThread')).toHaveLength(1)
  })

  test('保留中のスレッドが無い・取り出しに失敗しても、通常どおり起動する', async () => {
    const s = useThreadStore()
    s.start()
    await until(() => callsOf('TakePendingOpenThread').length === 1)
    expect(s.current).toBeUndefined()
    expect(useInboxStore().selectedRef).toBeUndefined()

    s.stop()
    ctl.failures.TakePendingOpenThread = appError('INTERNAL')
    s.start()
    await until(() => callsOf('TakePendingOpenThread').length === 2)
    expect(s.current).toBeUndefined()
  })

  test('一覧の選択に追従して open／close する', async () => {
    const inbox = useInboxStore()
    const s = useThreadStore()
    s.start()
    inbox.select(REF)
    await until(() => s.current !== undefined)
    inbox.select(undefined)
    await until(() => s.currentRef === undefined)
    expect(s.current).toBeUndefined()
  })

  test('start は冪等（購読が二重にならない）', () => {
    const s = useThreadStore()
    s.start()
    const first = handlers.get('sync:updated')
    s.start()
    expect(handlers.get('sync:updated')).toBe(first)
  })
})
