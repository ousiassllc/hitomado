import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import {
  appError,
  callsOf,
  ctl,
  installMocks,
  REF,
  resetState,
  restoreMocks,
  until,
} from '../thread/threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../../stores/inbox')
const { useUiStore } = await import('../../stores/ui')

function summary(threadId: string, extra: Partial<ThreadSummary> = {}): ThreadSummary {
  return {
    accountId: 'a1',
    threadId,
    subject: `件名 ${threadId}`,
    snippet: '',
    lastMessageAt: threadId === 't1' ? 20 : 10,
    lastFrom: { email: 'x@example.com' },
    messageCount: 1,
    isUnread: true,
    hasAttachment: false,
    inInbox: true,
    labels: null,
    ...extra,
  }
}

const T1 = summary('t1')
const T2 = summary('t2')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
  ctl.responses.Archive = []
  ctl.responses.Trash = []
  ctl.responses.Unarchive = [T1]
  ctl.responses.Untrash = [T1]
  // サーバーの状態: 取り消されていないアーカイブ・ゴミ箱の分だけ t1 が受信箱から外れている
  ctl.responses.ListInbox = () => {
    const removed =
      callsOf('Archive').length +
      callsOf('Trash').length -
      callsOf('Unarchive').length -
      callsOf('Untrash').length
    return { items: removed > 0 ? [T2] : [T1, T2], failures: [] }
  }
})

async function loaded() {
  const inbox = useInboxStore()
  await inbox.load()
  return inbox
}

describe('アーカイブ・ゴミ箱のスナックバー（元に戻す）', () => {
  test('アーカイブ成功の直後に「元に戻す」つきのスナックバーが出て、戻すと一覧と Gmail が元に戻る', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    expect(await inbox.archive([REF])).toBe(true)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t2'])
    expect(ui.toast?.message).toBe('アーカイブしました。')
    expect(ui.toast?.action?.label).toBe('元に戻す')

    ui.toast?.action?.run()
    await until(() => inbox.items.length === 2)
    expect(callsOf('Unarchive')[0].args).toEqual([[REF]])
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t1', 't2'])
    expect(ui.alerts).toHaveLength(0)
  })

  test('ゴミ箱成功の直後にスナックバーが出て、受信箱にあったスレッドは INBOX へ戻す', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    expect(await inbox.trash([REF])).toBe(true)
    expect(ui.toast?.message).toBe('ゴミ箱へ移動しました。')

    ui.toast?.action?.run()
    await until(() => callsOf('Untrash').length === 1)
    expect(callsOf('Untrash')[0].args).toEqual([[REF], true])
    await until(() => inbox.items.length === 2)
  })

  test('検索結果で受信箱にないスレッドをゴミ箱から戻すときは INBOX に入れない', async () => {
    const archived = summary('t1', { inInbox: false })
    ctl.responses.Search = { items: [archived], failures: [] }
    ctl.responses.Untrash = [archived]
    const inbox = useInboxStore()
    await inbox.runSearch('is:archived')
    const ui = useUiStore()
    expect(await inbox.trash([REF])).toBe(true)
    ui.toast?.action?.run()
    await until(() => callsOf('Untrash').length === 1)
    expect(callsOf('Untrash')[0].args).toEqual([[REF], false])
    await until(() => inbox.items.length === 1)
  })

  test('複数件は件数で知らせる', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    await inbox.archive([REF, { accountId: 'a1', threadId: 't2' }])
    expect(ui.toast?.message).toBe('2 件をアーカイブしました。')
  })

  test('操作が失敗したらスナックバーは出さず、エラーアラートで知らせる', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    ctl.failures.Archive = appError('RATE_LIMITED', '混み合っています')
    expect(await inbox.archive([REF])).toBe(false)
    expect(ui.toast).toBeUndefined()
    expect(ui.alerts).toHaveLength(1)
    expect(ui.alerts[0].kind).toBe('error')
  })

  test('元に戻すのに失敗したら、エラーアラート（再試行つき）で知らせる', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    await inbox.archive([REF])
    ctl.failures.Unarchive = appError('RATE_LIMITED', '混み合っています')
    ui.toast?.action?.run()
    await until(() => ui.alerts.length === 1)
    expect(ui.alerts[0].kind).toBe('error')
    expect(ui.alerts[0].message).toBe('元に戻せませんでした。混み合っています')
    expect(ui.alerts[0].retry).toBeDefined()

    delete ctl.failures.Unarchive
    ui.alerts[0].retry?.()
    await until(() => callsOf('Unarchive').length === 2)
    await until(() => inbox.items.length === 2)
  })

  test('既読・未読にはスナックバーを出さない', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    ctl.responses.MarkRead = [{ ...T1, isUnread: false }]
    await inbox.markRead([REF], true)
    expect(ui.toast).toBeUndefined()
  })
})
