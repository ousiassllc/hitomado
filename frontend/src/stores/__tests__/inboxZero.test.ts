import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import { appError, callsOf, until } from '../../components/thread/threadTestKit'
import {
  AI_OFF,
  aiCtl,
  cand,
  INBOX_ZERO_MAX,
  NONE,
  restoreInboxZeroMocks,
  setup,
} from './inboxZeroKit'

afterAll(restoreInboxZeroMocks)

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('Inbox Zero: 押せるか（#314・#377）', () => {
  test('理由がすべてオフ・一覧が空・使う AI が無いときは、それぞれの理由を返す', async () => {
    const { zero } = await setup({ keyed: false })
    expect(zero.blockedReason).toContain('Inbox Zero で使う AI が選ばれていません')

    zero.reasons = { ...NONE }
    expect(zero.blockedReason).toContain('すべてオフ')
  })

  test('返信済み・カレンダーだけがオンなら、AI を選んでいなくても押せる。一覧が空なら押せない', async () => {
    const { zero } = await setup({ keyed: false })
    zero.reasons = { ...NONE, replied: true }
    expect(zero.blockedReason).toBeUndefined()
    zero.reasons = { ...NONE, calendar: true }
    expect(zero.blockedReason).toBeUndefined()

    const empty = await setup({ count: 0 })
    expect(empty.zero.blockedReason).toContain('メールがありません')
  })

  test('Inbox Zero に選んだ AI（Jev 以外も）がつながっていれば押せる。選んだ AI がつながっていないときは、ほかの AI があっても押せない', async () => {
    const openai = await setup({ selected: 'openai', providers: ['openai'] })
    expect(openai.zero.selectedAI).toBe('openai')
    expect(openai.zero.blockedReason).toBeUndefined()

    const lost = await setup({ selected: 'openai', providers: ['typesafe', 'anthropic'] })
    expect(lost.zero.selectedAI).toBe('openai')
    expect(lost.zero.blockedReason).toContain('OpenAI 互換 がつながっていません')

    const gpt = await setup({ selected: 'chatgpt', providers: [] })
    expect(gpt.zero.blockedReason).toContain('ChatGPT がつながっていません')
    if (gpt.settings.aiConfig) gpt.settings.aiConfig.chatgptSignedIn = true
    expect(gpt.zero.blockedReason).toBeUndefined()
  })

  test('未選択のとき、Jev のキーがあれば Jev（互換）。無ければ選ぶよう案内する', async () => {
    const legacy = await setup()
    expect(legacy.zero.selectedAI).toBe('typesafe')
    expect(legacy.zero.blockedReason).toBeUndefined()

    const none = await setup({ providers: ['openai'] })
    expect(none.zero.selectedAI).toBeUndefined()
    expect(none.zero.blockedReason).toContain('Inbox Zero で使う AI が選ばれていません')
  })

  test('押せないときにボタンを押すと、理由をトーストで出し、AI へは何も送らない', async () => {
    const { zero, ui } = await setup({ keyed: false })
    await zero.start()
    expect(ui.toast?.message).toContain('Inbox Zero で使う AI が選ばれていません')
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

  test('同意の対象は、Inbox Zero に選んだ AI と送信先（OpenAI 互換は保存済みの Base URL）', async () => {
    const { zero } = await setup({
      consented: false,
      selected: 'openai',
      providers: ['openai'],
      baseUrl: 'https://llm.example/v1',
    })
    aiCtl.find = { candidates: [cand(1)], examined: 3, skipped: 0 }
    await zero.start()
    expect(zero.phase).toBe('consent')
    expect(zero.consentTarget).toEqual({ provider: 'openai', baseUrl: 'https://llm.example/v1' })
    await zero.agree()
    expect(callsOf('RecordAIConsent')[0].args).toEqual(['openai', 'https://llm.example/v1'])
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

  test('返信済み・カレンダーだけがオンなら AI へ送らないので、同意は求めない', async () => {
    const { zero } = await setup({ consented: false })
    zero.reasons = { ...AI_OFF }
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
