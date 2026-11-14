import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import type { DraftSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import {
  appError,
  calls,
  count,
  h,
  installMocks,
  names,
  reset,
  restoreMocks,
  session,
  setup,
  useAccountsStore,
  useDraftsStore,
  useInboxStore,
  useUiStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

const flush = () => new Promise<void>((r) => setTimeout(r, 0))

function draft(over: Partial<DraftSummary> = {}): DraftSummary {
  return {
    accountId: 'a1',
    draftId: 'd1',
    threadId: 'th1',
    updatedAt: 1_700_000_000_000,
    to: [{ name: '花子', email: 'hanako@example.com' }],
    subject: '打ち合わせ',
    snippet: '来週の日程です',
    isReply: false,
    ...over,
  }
}

function listing(items: DraftSummary[], over: Record<string, unknown> = {}) {
  h.responses.ListDrafts = { items, failures: [], truncated: false, ...over }
}

describe('drafts store（下書きの一覧。#366）', () => {
  test('開くと ListDrafts を呼び、絞り込み中のアカウントだけ読む。宛先の null は空にする', async () => {
    setup()
    useInboxStore().filter = { accountIds: ['a2'], unreadOnly: false }
    listing([draft({ to: null as unknown as DraftSummary['to'] })], { truncated: true })
    const drafts = useDraftsStore()
    drafts.show()
    await flush()
    expect(drafts.active).toBe(true)
    expect(calls.find((c) => c.name === 'ListDrafts')?.args).toEqual([['a2']])
    expect(drafts.items).toHaveLength(1)
    expect(drafts.items[0].to).toEqual([])
    expect(drafts.truncated).toBe(true)
    expect(drafts.loading).toBe(false)
  })

  test('絞り込みを変えると読み直す。閉じている間は読まない', async () => {
    setup()
    listing([draft()])
    const drafts = useDraftsStore()
    const inbox = useInboxStore()
    inbox.filter = { accountIds: ['a1'], unreadOnly: false }
    await flush()
    expect(count('ListDrafts')).toBe(0)
    drafts.show()
    await flush()
    inbox.filter = { accountIds: ['a2'], unreadOnly: false }
    await flush()
    expect(count('ListDrafts')).toBe(2)
    drafts.hide()
    inbox.filter = { accountIds: [], unreadOnly: false }
    await flush()
    expect(count('ListDrafts')).toBe(2)
  })

  test('古い読み込みの応答は、新しい読み込みの結果を上書きしない', async () => {
    setup()
    const drafts = useDraftsStore()
    let release: () => void = () => {}
    h.gates.ListDrafts = new Promise<void>((r) => {
      release = r
    })
    listing([draft({ draftId: 'old' })])
    const first = drafts.load()
    delete h.gates.ListDrafts
    listing([draft({ draftId: 'new' })])
    await drafts.load()
    release()
    await first
    expect(drafts.items.map((d) => d.draftId)).toEqual(['new'])
    expect(drafts.loading).toBe(false)
  })

  test('読み込みの最中に破棄した行は、破棄より前の状態の応答でも復活しない', async () => {
    setup()
    const drafts = useDraftsStore()
    listing([draft(), draft({ draftId: 'd2' })])
    drafts.show()
    await flush()
    let release: () => void = () => {}
    h.gates.ListDrafts = new Promise<void>((r) => {
      release = r
    })
    const reloading = drafts.load()
    delete h.gates.ListDrafts
    h.responses.DiscardDraft = undefined
    expect(await drafts.discard(draft())).toBe(true)
    release()
    await reloading
    expect(drafts.items.map((d) => d.draftId)).toEqual(['d2'])
  })

  test('新しい読み込み・一覧を閉じたときは、進行中の読み込みを中止する（Go 側を走らせ続けない）', async () => {
    setup()
    const drafts = useDraftsStore()
    const cancelled: string[] = []
    h.cancelHooks.ListDrafts = () => cancelled.push('c')
    h.gates.ListDrafts = new Promise<void>(() => {})
    void drafts.load()
    void drafts.load()
    expect(cancelled).toHaveLength(1)
    drafts.hide()
    expect(cancelled).toHaveLength(2)
    expect(drafts.loading).toBe(false)
  })

  test('新規の下書きは、送信元アカウントつきの new セッションで開く', async () => {
    const { compose } = setup()
    h.responses.OpenCompose = session({ mode: 'new', senderLocked: false, accountId: 'a2' })
    const drafts = useDraftsStore()
    expect(await drafts.open(draft({ accountId: 'a2', draftId: 'd9' }))).toBe(true)
    const open = calls.find((c) => c.name === 'OpenCompose')
    expect(open?.args[0]).toEqual({ mode: 'new', accountId: 'a2', draftId: 'd9' })
    expect(compose.session?.mode).toBe('new')
  })

  test('返信の下書きは、そのスレッドを選び、返信として開く（F-12 の復元と同じ）', async () => {
    const { compose } = setup()
    h.responses.OpenCompose = session({ mode: 'reply_all' })
    const drafts = useDraftsStore()
    await drafts.open(draft({ isReply: true, threadId: 'th7' }))
    expect(useInboxStore().selectedRef).toEqual({ accountId: 'a1', threadId: 'th7' })
    expect(calls.find((c) => c.name === 'OpenCompose')?.args[0]).toEqual({
      mode: 'reply_all',
      ref: { accountId: 'a1', threadId: 'th7' },
      draftId: 'd1',
    })
    expect(compose.isOpenForThread({ accountId: 'a1', threadId: 'th7' })).toBe(true)
  })

  test('作成セッションを閉じる・送信する・破棄すると、一覧を読み直す。保存のたびには読み直さない', async () => {
    const { compose } = setup()
    h.responses.OpenCompose = session({ mode: 'new', senderLocked: false, draftId: 'd1' })
    listing([draft()])
    const drafts = useDraftsStore()
    drafts.show()
    await flush()
    await drafts.open(draft())
    await flush()
    expect(count('ListDrafts')).toBe(1)
    expect(await compose.close()).toBe(true)
    await flush()
    expect(count('ListDrafts')).toBe(2)
  })

  describe('破棄', () => {
    test('開いていない下書きは DiscardDraft で消し、一覧から外す。ほかの行は残る', async () => {
      setup()
      listing([draft(), draft({ draftId: 'd2', accountId: 'a2' })])
      const drafts = useDraftsStore()
      drafts.show()
      await flush()
      h.responses.DiscardDraft = undefined
      expect(await drafts.discard(draft())).toBe(true)
      expect(calls.find((c) => c.name === 'DiscardDraft')?.args[0]).toEqual({
        accountId: 'a1',
        draftId: 'd1',
        threadId: 'th1',
      })
      expect(drafts.items.map((d) => d.draftId)).toEqual(['d2'])
      expect(useUiStore().toast?.message).toBeDefined()
      expect(names()).not.toContain('DiscardCompose')
    })

    test('開いている作成セッションの下書きは、セッションごと破棄する（DiscardCompose）', async () => {
      const { compose } = setup()
      h.responses.OpenCompose = session({ mode: 'new', senderLocked: false, draftId: 'd1' })
      const drafts = useDraftsStore()
      await drafts.open(draft())
      expect(drafts.isOpen(draft())).toBe(true)
      expect(await drafts.discard(draft())).toBe(true)
      expect(names()).toContain('DiscardCompose')
      expect(names()).not.toContain('DiscardDraft')
      expect(compose.session).toBeUndefined()
    })

    test('失敗したら一覧に残し、理由つきのエラーを出す', async () => {
      setup()
      listing([draft()])
      const drafts = useDraftsStore()
      drafts.show()
      await flush()
      h.failures.DiscardDraft = appError('GMAIL_ERROR')
      expect(await drafts.discard(draft())).toBe(false)
      expect(drafts.items).toHaveLength(1)
      const alert = useUiStore().alerts.at(-1)
      expect(alert?.message).toContain('下書きを破棄できませんでした')
      expect(alert?.message).toContain('msg-GMAIL_ERROR')
    })
  })

  describe('取得の失敗', () => {
    test('一部のアカウントの失敗は、ほかの下書きを出したまま、アカウント名つきで再試行を出す', async () => {
      setup()
      listing([draft()], { failures: [{ accountId: 'a2', error: appError('GMAIL_ERROR') }] })
      const drafts = useDraftsStore()
      drafts.show()
      await flush()
      expect(drafts.items).toHaveLength(1)
      const alert = useUiStore().alerts.at(-1)
      expect(alert?.message).toBe('B の下書きを読み込めませんでした。')
      alert?.retry?.()
      await flush()
      expect(count('ListDrafts')).toBe(2)
    })

    test('要再接続は、アカウントを要再接続にして、個別のエラーは出さない（共通のバナーに任せる）', async () => {
      setup()
      listing([], { failures: [{ accountId: 'a2', error: appError('NEEDS_REAUTH') }] })
      const drafts = useDraftsStore()
      drafts.show()
      await flush()
      expect(useUiStore().alerts).toHaveLength(0)
      expect(useAccountsStore().statusOf('a2')).toBe('needs_reauth')
    })

    test('全体の失敗は一覧を変えずにエラーを出す。オフラインは出さない', async () => {
      setup()
      listing([draft()])
      const drafts = useDraftsStore()
      drafts.show()
      await flush()
      h.failures.ListDrafts = appError('GMAIL_ERROR')
      await drafts.load()
      expect(drafts.items).toHaveLength(1)
      expect(useUiStore().alerts.at(-1)?.message).toBe('下書きを読み込めませんでした。')
      useUiStore().alerts.splice(0)
      h.failures.ListDrafts = appError('OFFLINE')
      await drafts.load()
      expect(useUiStore().alerts).toHaveLength(0)
    })
  })
})
