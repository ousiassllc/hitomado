import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  agentItem,
  agentOverview,
  appError,
  callsOf,
  deferred,
  failures,
  gates,
  handlers,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
  until,
} from '../../../test/inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAgentStore } = await import('../agent')
const { useUiStore } = await import('../ui')

beforeEach(resetKit)

const settingsOf = (over: Record<string, unknown> = {}) => ({
  enabled: true,
  mascotEnabled: false,
  aiReady: true,
  ...over,
})

describe('agent ストアの読み込み', () => {
  test('start で設定と一覧を読み、冪等。オフの間は pendingCount を 0 にする', async () => {
    responses.ListAgentItems = agentOverview([
      agentItem('i1'),
      agentItem('i2', { status: 'asking' }),
    ])
    const store = useAgentStore()
    await store.start()
    await store.start()
    expect(callsOf('GetAgentSettings')).toHaveLength(1)
    expect(store.items.map((i) => i.id)).toEqual(['i1', 'i2'])
    expect(store.pendingCount).toBe(0) // オフ

    responses.GetAgentSettings = settingsOf()
    await store.load()
    expect(store.pendingCount).toBe(2)
  })

  test('items が null でも空配列として扱う', async () => {
    responses.ListAgentItems = { pendingCount: 0, items: null }
    const store = useAgentStore()
    await store.load()
    expect(store.items).toEqual([])
  })

  test('agent:updated で読み直し、agent:open-panel でパネルを開く', async () => {
    const store = useAgentStore()
    const ui = useUiStore()
    await store.start()
    expect(store.pendingCount).toBe(0)

    responses.GetAgentSettings = settingsOf()
    responses.ListAgentItems = agentOverview([agentItem('i1')])
    handlers.get('agent:updated')?.({ pendingCount: 1, enabled: true, mascotEnabled: false })
    await until(() => store.pendingCount === 1)

    expect(ui.agentOpen).toBe(false)
    handlers.get('agent:open-panel')?.(undefined)
    expect(ui.agentOpen).toBe(true)

    store.stop()
    expect(handlers.has('agent:updated')).toBe(false)
    expect(handlers.has('agent:open-panel')).toBe(false)
  })

  test('読み込みの失敗はアラートで知らせ、投げ直さない', async () => {
    failures.GetAgentSettings = appError('INTERNAL')
    const ui = useUiStore()
    await useAgentStore().load()
    expect(ui.alerts.map((a) => a.message)).toEqual(['err INTERNAL'])
  })

  test('飛行中の古い読み込みは、新しい設定を上書きしない', async () => {
    const gate = deferred()
    gates.GetAgentSettings = gate.p
    const store = useAgentStore()
    const pending = store.load() // 古い設定（オフ）を持ったまま止まる
    responses.SetAgentMascot = settingsOf({ mascotEnabled: true })
    await store.setMascot(true)
    delete gates.GetAgentSettings
    gate.resolve()
    await pending
    expect(store.settings.mascotEnabled).toBe(true)
    expect(store.enabled).toBe(true)
  })
})

describe('agent ストアの設定', () => {
  test('setEnabled は同意（agreed）を渡して反映し、一覧を読み直す', async () => {
    responses.SetAgentEnabled = settingsOf()
    responses.GetAgentSettings = settingsOf()
    responses.ListAgentItems = agentOverview([agentItem('i1')])
    const store = useAgentStore()
    await store.setEnabled(true, true)
    expect(callsOf('SetAgentEnabled')[0]?.args).toEqual([true, true])
    expect(store.enabled).toBe(true)
    expect(store.pendingCount).toBe(1)
  })

  test('setEnabled の失敗は AppError のまま投げ、設定は変えない（呼び出し側が案内する）', async () => {
    failures.SetAgentEnabled = appError('AI_NOT_CONFIGURED')
    const store = useAgentStore()
    await expect(store.setEnabled(true, true)).rejects.toMatchObject({ code: 'AI_NOT_CONFIGURED' })
    expect(store.enabled).toBe(false)
  })

  test('setMascot はマスコットの設定を反映する', async () => {
    responses.SetAgentMascot = settingsOf({ mascotEnabled: true })
    const store = useAgentStore()
    await store.setMascot(true)
    expect(callsOf('SetAgentMascot')[0]?.args).toEqual([true])
    expect(store.settings.mascotEnabled).toBe(true)
  })
})

describe('agent ストアの項目の操作', () => {
  async function started(items: ReturnType<typeof agentItem>[]) {
    responses.GetAgentSettings = settingsOf()
    responses.ListAgentItems = agentOverview(items)
    const store = useAgentStore()
    await store.load()
    return store
  }

  test('answer は答えを渡し、返った 1 件で行を置き換えて件数を合わせる', async () => {
    const store = await started([
      agentItem('i1', { status: 'asking', draftId: undefined, questions: ['日程は？'] }),
    ])
    responses.AnswerAgentQuestions = agentItem('i1', { status: 'ready' })
    expect(await store.answer('i1', ['来週'])).toBe(true)
    expect(callsOf('AnswerAgentQuestions')[0]?.args).toEqual(['i1', ['来週']])
    expect(store.items[0]?.status).toBe('ready')
    expect(store.pendingCount).toBe(1)
  })

  test('defer・confirm・reconsider は対応するバインディングを呼び、件数を合わせる', async () => {
    const store = await started([
      agentItem('i1'),
      agentItem('i2'),
      agentItem('i3', { status: 'skipped' }),
    ])
    expect(store.pendingCount).toBe(2)
    responses.DeferAgentItem = agentItem('i1', { status: 'later' })
    responses.ConfirmAgentItem = agentItem('i2', { status: 'done' })
    responses.ReconsiderAgentItem = agentItem('i3', { status: 'ready' })
    await store.defer('i1')
    expect(store.pendingCount).toBe(1)
    await store.confirm('i2')
    expect(store.pendingCount).toBe(0)
    await store.reconsider('i3')
    expect(store.pendingCount).toBe(1)
    expect(callsOf('DeferAgentItem')[0]?.args).toEqual(['i1'])
    expect(callsOf('ConfirmAgentItem')[0]?.args).toEqual(['i2'])
    expect(callsOf('ReconsiderAgentItem')[0]?.args).toEqual(['i3'])
    expect(store.items.map((i) => i.status)).toEqual(['later', 'done', 'ready'])
  })

  test('処理中の同じ項目は受け付けず（二重送信しない）、終われば解ける', async () => {
    const store = await started([agentItem('i1')])
    const gate = deferred()
    gates.DeferAgentItem = gate.p
    responses.DeferAgentItem = agentItem('i1', { status: 'later' })
    const first = store.defer('i1')
    expect(store.isBusy('i1')).toBe(true)
    expect(await store.defer('i1')).toBe(false)
    gate.resolve()
    expect(await first).toBe(true)
    expect(callsOf('DeferAgentItem')).toHaveLength(1)
    expect(store.isBusy('i1')).toBe(false)
  })

  test('失敗はアラートで知らせて false を返し、行は変えず処理中も解く', async () => {
    const store = await started([agentItem('i1', { status: 'asking', questions: ['q'] })])
    failures.AnswerAgentQuestions = appError('AI_FAILED')
    const ui = useUiStore()
    expect(await store.answer('i1', ['a'])).toBe(false)
    expect(ui.alerts.map((a) => a.message)).toEqual(['err AI_FAILED'])
    expect(store.items[0]?.status).toBe('asking')
    expect(store.isBusy('i1')).toBe(false)
  })
})
