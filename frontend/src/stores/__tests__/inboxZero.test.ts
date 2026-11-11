import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realIndex from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  InboxZeroCandidate,
  ThreadSummary,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import {
  appError,
  callsOf,
  ctl,
  installMocks,
  calls as kitCalls,
  resetState,
  restoreMocks,
  until,
} from '../../components/thread/threadTestKit'

// Jev の実 API は呼ばない。ストア（inboxZero.ts）が import する Go のバインディング（index の AIService）をモックする。
// 同意の保存（settings.recordAIConsent）は、すでに読み込まれた settings ストアのメソッドを setup で差し替える。
const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
installMocks()
const aiCtl: { find: unknown; failures: Record<string, unknown> } = {
  find: undefined,
  failures: {},
}
function aiBinding(name: string) {
  return async (...args: unknown[]) => {
    kitCalls.push({ name, args })
    if (name in aiCtl.failures) throw aiCtl.failures[name]
    return typeof aiCtl.find === 'function'
      ? (aiCtl.find as (...a: unknown[]) => unknown)(...args)
      : aiCtl.find
  }
}
mock.module(B, () => ({
  ...realIndex,
  AIService: {
    FindInboxZeroCandidates: aiBinding('FindInboxZeroCandidates'),
    CancelGenerate: aiBinding('CancelGenerate'),
  },
}))
afterAll(() => {
  restoreMocks()
  mock.module(B, () => ({ ...realIndex }))
})

const { useInboxStore } = await import('../inbox')
const { useInboxZeroStore, INBOX_ZERO_MAX } = await import('../inboxZero')
const { useSettingsStore } = await import('../settings')
const { useUiStore } = await import('../ui')

function summary(n: number): ThreadSummary {
  return {
    accountId: 'a1',
    threadId: `t${n}`,
    subject: `件名 ${n}`,
    snippet: '',
    lastMessageAt: 1000 - n,
    lastFrom: { email: `x${n}@example.com` },
    messageCount: 1,
    isUnread: true,
    hasAttachment: false,
    inInbox: true,
    labels: null,
  }
}

const cand = (n: number, reason = 'ad'): InboxZeroCandidate => ({
  ref: { accountId: 'a1', threadId: `t${n}` },
  from: `送信者 ${n}`,
  subject: `件名 ${n}`,
  reason,
})

const ALL_ON = { ad: true, notification: true, newsletter: true, replied: true }

async function setup(opts: { count?: number; consented?: boolean; keyed?: boolean } = {}) {
  const { count = 3, consented = true, keyed = true } = opts
  setActivePinia(createPinia())
  resetState()
  aiCtl.find = undefined
  aiCtl.failures = {}
  const all = Array.from({ length: count }, (_, i) => summary(i + 1))
  // アーカイブしたものは、あとの再取得（アーカイブ後の reload）では返らない。
  ctl.responses.ListInbox = () => ({
    items: all.filter(
      (t) =>
        !callsOf('Archive').some((c) =>
          (c.args[0] as { threadId: string }[]).some((r) => r.threadId === t.threadId),
        ),
    ),
    failures: [],
  })
  ctl.responses.Archive = []
  ctl.responses.Unarchive = [summary(1)]
  const inbox = useInboxStore()
  await inbox.load()
  const settings = useSettingsStore()
  useInboxZeroStore().reasons = { ...ALL_ON }
  settings.aiConfig = {
    provider: 'chatgpt',
    model: '',
    configured: false,
    hasApiKey: false,
    apiKeyProviders: keyed ? ['typesafe'] : [],
    consentedProviders: consented ? ['typesafe'] : [],
    chatgptSignedIn: false,
    chatgptAccounts: [],
  }
  settings.recordAIConsent = async (provider: string, baseUrl = '') => {
    kitCalls.push({ name: 'RecordAIConsent', args: [provider, baseUrl] })
    if (settings.aiConfig) {
      settings.aiConfig = { ...settings.aiConfig, consentedProviders: [provider] }
    }
  }
  return { inbox, settings, zero: useInboxZeroStore(), ui: useUiStore() }
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('Inbox Zero: 押せるか（#314）', () => {
  test('理由がすべてオフ・一覧が空・Jev の API キー未登録のときは、それぞれの理由を返す', async () => {
    const { zero } = await setup({ keyed: false })
    expect(zero.blockedReason).toContain('API キーが未登録')

    zero.reasons = {
      ad: false,
      notification: false,
      newsletter: false,
      replied: false,
    }
    expect(zero.blockedReason).toContain('すべてオフ')
  })

  test('返信済みだけがオンなら、Jev のキーが無くても押せる。一覧が空なら押せない', async () => {
    const { zero } = await setup({ keyed: false })
    zero.reasons = { ad: false, notification: false, newsletter: false, replied: true }
    expect(zero.blockedReason).toBeUndefined()

    const empty = await setup({ count: 0 })
    expect(empty.zero.blockedReason).toContain('メールがありません')
  })

  test('押せないときにボタンを押すと、理由をトーストで出し、Jev へは何も送らない', async () => {
    const { zero, ui } = await setup({ keyed: false })
    await zero.start()
    expect(ui.toast?.message).toContain('API キーが未登録')
    expect(callsOf('FindInboxZeroCandidates')).toHaveLength(0)
    expect(zero.phase).toBe('idle')
  })
})

describe('Inbox Zero: 同意（初回）', () => {
  test('同意前は送らない。同意すると保存してから判断を始める', async () => {
    const { zero } = await setup({ consented: false })
    aiCtl.find = { candidates: [cand(1)], examined: 3, skipped: 0 }
    await zero.start()
    expect(zero.phase).toBe('consent')
    expect(callsOf('FindInboxZeroCandidates')).toHaveLength(0)

    await zero.agree()
    expect(callsOf('RecordAIConsent')[0].args).toEqual(['typesafe', ''])
    expect(callsOf('FindInboxZeroCandidates')).toHaveLength(1)
    expect(zero.phase).toBe('review')
  })

  test('同意のキャンセルでは何も送らず、同意も保存しない', async () => {
    const { zero } = await setup({ consented: false })
    await zero.start()
    zero.cancel()
    expect(zero.phase).toBe('idle')
    expect(callsOf('RecordAIConsent')).toHaveLength(0)
    expect(callsOf('FindInboxZeroCandidates')).toHaveLength(0)
  })

  test('返信済みだけがオンなら Jev へ送らないので、同意は求めない', async () => {
    const { zero } = await setup({ consented: false })
    zero.reasons = { ad: false, notification: false, newsletter: false, replied: true }
    aiCtl.find = { candidates: [cand(1, 'replied')], examined: 3, skipped: 0 }
    await zero.start()
    expect(zero.phase).toBe('review')
    expect(callsOf('RecordAIConsent')).toHaveLength(0)
  })
})

describe('Inbox Zero: 判断と確認の一覧', () => {
  test('今のタブに読み込まれているメールだけを、100 件まで渡す', async () => {
    const { zero } = await setup({ count: INBOX_ZERO_MAX + 20 })
    aiCtl.find = { candidates: [], examined: INBOX_ZERO_MAX, skipped: 0 }
    await zero.start()
    const [refs, requestId] = callsOf('FindInboxZeroCandidates')[0].args as [unknown[], string]
    expect(refs).toHaveLength(INBOX_ZERO_MAX)
    expect(refs[0]).toEqual({ accountId: 'a1', threadId: 't1' })
    expect(requestId).toBeTruthy()
  })

  test('候補が無ければ、一覧は出さずに「見つかりませんでした」と知らせる', async () => {
    const { zero, ui } = await setup()
    aiCtl.find = { candidates: [], examined: 3, skipped: 0 }
    await zero.start()
    expect(zero.phase).toBe('idle')
    expect(ui.toast?.message).toContain('見つかりませんでした')
  })

  test('一覧は最初は全部チェック済み。チェックを外したものはアーカイブされず残り、Yes のあとは件数と「元に戻す」が出る', async () => {
    const { inbox, zero, ui } = await setup()
    aiCtl.find = { candidates: [cand(1), cand(2), cand(3, 'newsletter')], examined: 3, skipped: 0 }
    await zero.start()
    expect(zero.phase).toBe('review')
    expect(zero.checkedCandidates).toHaveLength(3)

    zero.toggle('a1:t2') // 残したいものはチェックを外す
    expect(zero.checkedCandidates.map((c) => c.ref.threadId)).toEqual(['t1', 't3'])
    await zero.confirm()

    expect(callsOf('Archive')).toHaveLength(1)
    expect(callsOf('Archive')[0].args).toEqual([
      [
        { accountId: 'a1', threadId: 't1' },
        { accountId: 'a1', threadId: 't3' },
      ],
    ])
    expect(inbox.items.map((t) => t.threadId)).toEqual(['t2']) // 外したものは受信トレイに残る
    expect(ui.toast?.message).toBe('2 件をアーカイブしました。')
    expect(ui.toast?.action?.label).toBe('元に戻す')
    expect(zero.phase).toBe('idle')
  })

  test('元に戻すと、アーカイブしたものを受信トレイへ戻す', async () => {
    const { zero, ui } = await setup()
    aiCtl.find = { candidates: [cand(1)], examined: 3, skipped: 0 }
    await zero.start()
    await zero.confirm()
    ui.toast?.action?.run()
    await until(() => callsOf('Unarchive').length === 1)
    expect(callsOf('Unarchive')[0].args).toEqual([[{ accountId: 'a1', threadId: 't1' }]])
  })

  test('「やめる（No）」では何も変えない（アーカイブしない・一覧もそのまま）', async () => {
    const { inbox, zero, ui } = await setup()
    aiCtl.find = { candidates: [cand(1), cand(2)], examined: 3, skipped: 0 }
    await zero.start()
    zero.cancel()
    expect(zero.phase).toBe('idle')
    expect(callsOf('Archive')).toHaveLength(0)
    expect(inbox.items).toHaveLength(3)
    expect(ui.toast).toBeUndefined()
  })

  test('チェックをすべて外した Yes は何もしない', async () => {
    const { zero } = await setup()
    aiCtl.find = { candidates: [cand(1)], examined: 3, skipped: 0 }
    await zero.start()
    zero.toggle('a1:t1')
    await zero.confirm()
    expect(callsOf('Archive')).toHaveLength(0)
    expect(zero.phase).toBe('review')
  })

  test('判断の最中にやめると、Go の呼び出しを中止し、あとから返った結果は捨てる', async () => {
    const { zero } = await setup()
    const gate = Promise.withResolvers<unknown>()
    aiCtl.find = () => gate.promise
    const running = zero.start()
    await until(() => callsOf('FindInboxZeroCandidates').length === 1)
    expect(zero.phase).toBe('checking')
    const requestId = callsOf('FindInboxZeroCandidates')[0].args[1]

    zero.cancel()
    expect(callsOf('CancelGenerate')[0].args).toEqual([requestId])
    gate.resolve({ candidates: [cand(1)], examined: 3, skipped: 0 })
    await running
    expect(zero.phase).toBe('idle')
    expect(zero.candidates).toHaveLength(0)
  })

  test('判断中は二度押ししても 2 回目は始まらない', async () => {
    const { zero } = await setup()
    const gate = Promise.withResolvers<unknown>()
    aiCtl.find = () => gate.promise
    const first = zero.start()
    await until(() => zero.phase === 'checking')
    await zero.start()
    expect(callsOf('FindInboxZeroCandidates')).toHaveLength(1)
    gate.resolve({ candidates: [], examined: 3, skipped: 0 })
    await first
  })

  test('Go が同意を求めたら同意の画面へ戻り、ほかの失敗はアラートで知らせる', async () => {
    const { zero, ui } = await setup()
    aiCtl.failures.FindInboxZeroCandidates = appError('AI_CONSENT_REQUIRED', '同意が必要です。')
    await zero.start()
    expect(zero.phase).toBe('consent')
    zero.cancel()

    aiCtl.failures.FindInboxZeroCandidates = appError('AI_FAILED', 'AI が失敗しました。')
    await zero.start()
    expect(zero.phase).toBe('idle')
    expect(ui.alerts.map((a) => a.message)).toContain('AI が失敗しました。')
  })
})
