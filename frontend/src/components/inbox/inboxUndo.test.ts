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

  test('戻したスレッドは、先頭ページの再取得を待たずに一覧へ差し戻す', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    // 再取得を止めておき、差し戻しだけで一覧が戻ることを確かめる
    const gate = Promise.withResolvers<void>()
    ctl.responses.ListInbox = async () => {
      await gate.promise
      return { items: [T2], failures: [] }
    }
    await inbox.archive([REF])
    ui.toast?.action?.run()
    await until(() => inbox.items.length === 2)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t1', 't2'])
    gate.resolve()
  })

  test('中ほどの行を戻すと、先頭ページの再取得を待たずに元の位置へ戻る', async () => {
    const t0 = summary('t0', { lastMessageAt: 30 })
    ctl.responses.ListInbox = () => ({ items: [t0, T1, T2], failures: [] })
    const inbox = await loaded()
    const ui = useUiStore()
    const gate = Promise.withResolvers<void>()
    ctl.responses.ListInbox = async () => {
      await gate.promise
      return { items: [t0, T2], failures: [] }
    }
    await inbox.archive([REF])
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t0', 't2'])
    ui.toast?.action?.run()
    await until(() => inbox.items.length === 3)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t0', 't1', 't2'])
    gate.resolve()
  })

  test('アーカイブ後に検索へ切り替えてから「元に戻す」を押しても、検索結果に条件外の行を入れない', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    await inbox.archive([REF])
    const undo = ui.toast?.action?.run
    ctl.responses.Search = { items: [T2], failures: [] }
    await inbox.runSearch('t2')
    const counted = callsOf('GetUnreadCounts').length
    undo?.()
    // 取り消しの最後の再取得（未読件数）まで済んでも、検索結果は変わらない
    await until(() => callsOf('GetUnreadCounts').length === counted + 1)
    expect(callsOf('Unarchive')).toHaveLength(1)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t2'])
  })

  test('アーカイブ後にアカウントの絞り込みを変えてから「元に戻す」を押しても、絞り込み外の行を入れない', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    await inbox.archive([REF])
    const undo = ui.toast?.action?.run
    ctl.responses.ListInbox = () => ({ items: [T2], failures: [] })
    await inbox.setFilter({ accountIds: ['a2'], unreadOnly: false })
    // 差し戻しの後の再取得を止めておき、差し戻しだけで一覧が変わらないことを確かめる
    const gate = Promise.withResolvers<void>()
    ctl.responses.ListInbox = async () => {
      await gate.promise
      return { items: [T2], failures: [] }
    }
    const listed = callsOf('ListInbox').length
    undo?.()
    await until(() => callsOf('ListInbox').length === listed + 1)
    expect(callsOf('Unarchive')).toHaveLength(1)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t2'])
    gate.resolve()
  })

  test('検索結果で受信箱にないスレッドのアーカイブは、取り消す対象が無いので「元に戻す」を出さない', async () => {
    ctl.responses.Search = { items: [summary('t1', { inInbox: false })], failures: [] }
    const inbox = useInboxStore()
    await inbox.runSearch('is:starred')
    const ui = useUiStore()
    expect(await inbox.archive([REF])).toBe(true)
    expect(ui.toast?.message).toBe('アーカイブしました。')
    expect(ui.toast?.action).toBeUndefined()
    expect(callsOf('Unarchive')).toHaveLength(0)
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
    ctl.responses.Untrash = [archived]
    // サーバーの状態: ゴミ箱にある間は、アーカイブ済みの検索結果に出ない
    ctl.responses.Search = () => ({
      items: callsOf('Trash').length > callsOf('Untrash').length ? [] : [archived],
      failures: [],
    })
    const inbox = useInboxStore()
    await inbox.runSearch('is:archived')
    const ui = useUiStore()
    expect(await inbox.trash([REF])).toBe(true)
    expect(inbox.items).toHaveLength(0)
    ui.toast?.action?.run()
    await until(() => callsOf('Untrash').length === 1)
    expect(callsOf('Untrash')[0].args).toEqual([[REF], false])
    // 検索結果は再取得されない（Search は最初の 1 回だけ）ので、戻った行は差し戻しの効果。受信箱には入れていない
    await until(() => inbox.items.length === 1)
    expect(callsOf('Search')).toHaveLength(1)
    expect(inbox.items[0].inInbox).toBe(false)
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

  test('元に戻すのに失敗したら、エラーアラート（再試行つき）で知らせる。再試行が成功したら一覧へ差し戻す', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    await inbox.archive([REF])
    // サーバーの状態: 取り消しが成功するまで t1 は受信箱に戻らない（失敗した Unarchive では戻らない）
    let undone = false
    ctl.responses.Unarchive = () => {
      undone = true
      return [T1]
    }
    ctl.responses.ListInbox = () => ({ items: undone ? [T1, T2] : [T2], failures: [] })
    ctl.failures.Unarchive = appError('RATE_LIMITED', '混み合っています')
    ui.toast?.action?.run()
    await until(() => ui.alerts.length === 1)
    expect(ui.alerts[0].kind).toBe('error')
    expect(ui.alerts[0].message).toBe('元に戻せませんでした。混み合っています')
    expect(ui.alerts[0].retry).toBeDefined()
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t2'])

    delete ctl.failures.Unarchive
    ui.alerts[0].retry?.()
    await until(() => callsOf('Unarchive').length === 2)
    await until(() => inbox.items.length === 2)
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t1', 't2'])
  })

  test('既読・未読にはスナックバーを出さない', async () => {
    const inbox = await loaded()
    const ui = useUiStore()
    ctl.responses.MarkRead = [{ ...T1, isUnread: false }]
    await inbox.markRead([REF], true)
    expect(ui.toast).toBeUndefined()
  })
})
