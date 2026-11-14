import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import {
  agentItem,
  agentOverview,
  callsOf,
  deferred,
  gates,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from '../../../test/inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAgentStore } = await import('../../stores/agent')
const { useComposeStore } = await import('../../stores/compose')
const { useInboxStore } = await import('../../stores/inbox')
const { useUiStore } = await import('../../stores/ui')
const { default: AgentPanel } = await import('../shell/AgentPanel.vue')

beforeEach(() => {
  resetKit()
  document.body.innerHTML = ''
})

type Item = ReturnType<typeof agentItem>

/** エージェントを（オンで）読み込んでから、パネルを開いて描く。 */
async function openPanel(items: Item[], enabled = true) {
  responses.GetAgentSettings = { enabled, mascotEnabled: false, aiReady: true }
  responses.ListAgentItems = agentOverview(items)
  await useAgentStore().load()
  const ui = useUiStore()
  ui.openAgent()
  const w = mount(AgentPanel, { attachTo: document.body })
  return { w, ui }
}

const card = (w: Awaited<ReturnType<typeof openPanel>>['w'], id: string) =>
  w.get(`[data-testid=agent-item-${id}]`)

describe('エージェントのパネル', () => {
  test('オフのときは説明と「設定でオンにする」だけを出し、押すと設定のエージェントのページを開く', async () => {
    const { w, ui } = await openPanel([agentItem('i1')], false)
    expect(w.find('[data-testid=agent-item-i1]').exists()).toBe(false)
    expect(w.get('[data-testid=agent-off]').text()).toContain('送信は必ず自分で押します')
    await w.get('[data-testid=agent-open-settings]').trigger('click')
    expect(ui.dialogs.settings).toBe(true)
    expect(ui.settingsTab).toBe('agent')
    w.unmount()
  })

  test('オンで何も無い（確認済みしか無い）ときは、空の案内を出す', async () => {
    const { w } = await openPanel([agentItem('i1', { status: 'done' })])
    expect(w.get('[data-testid=agent-empty]').text()).toContain('確認するものはありません')
    expect(w.find('[data-testid=agent-item-i1]').exists()).toBe(false)
    w.unmount()
  })

  test('状態ごとの区分に行を並べ、件名・差出人・理由・受信アカウントを出す。あとで・返信不要は折りたたみ、確認済みは出さない', async () => {
    const { w } = await openPanel([
      agentItem('r1'),
      agentItem('q1', {
        status: 'asking',
        draftId: undefined,
        draftPreview: '',
        questions: ['日程は？'],
        accountName: '私用',
        accountEmail: 'a2@example.com',
      }),
      agentItem('l1', { status: 'later' }),
      agentItem('s1', { status: 'skipped', draftId: undefined, reason: '広告メールです' }),
      agentItem('f1', { status: 'failed', draftId: undefined }),
      agentItem('d1', { status: 'done' }),
    ])
    const order = w.findAll('section[aria-label]').map((s) => s.attributes('aria-label'))
    expect(order).toEqual([
      '質問（1件）',
      '確認待ち（1件）',
      '失敗（1件）',
      'あとで（1件）',
      '返信不要と判断（1件）',
    ])
    expect(w.find('[data-testid=agent-item-d1]').exists()).toBe(false)

    const ready = card(w, 'r1')
    expect(ready.text()).toContain('件名-r1')
    expect(ready.text()).toContain('差出人: 山田')
    expect(ready.text()).toContain('宛先: 山田')
    expect(ready.get('[data-testid=agent-reason]').text()).toBe('日程の相談です')
    expect(ready.get('[data-testid=agent-preview]').text()).toBe('ご連絡ありがとうございます。')
    expect(ready.text()).toContain('仕事')
    expect(card(w, 'q1').text()).toContain('私用')
    expect(card(w, 's1').get('[data-testid=agent-reason]').text()).toBe('広告メールです')

    // あとで・返信不要は折りたたみ（details の中）
    expect(card(w, 'l1').element.closest('details')).not.toBeNull()
    expect(card(w, 's1').element.closest('details')).not.toBeNull()
    expect(ready.element.closest('details')).toBeNull()
    w.unmount()
  })

  test('質問は、すべて答えるまで送れず、答え（前後の空白を除く）を質問と同じ順で送る', async () => {
    const { w } = await openPanel([
      agentItem('q1', {
        status: 'asking',
        draftId: undefined,
        questions: ['日程は？', '金額は？'],
      }),
    ])
    responses.AnswerAgentQuestions = agentItem('q1', { status: 'ready' })
    const submit = () => w.get('[data-testid=agent-answer-submit]')
    const inputs = w.findAll('[data-testid=agent-answer]')
    expect(inputs).toHaveLength(2)
    expect(card(w, 'q1').text()).toContain('日程は？')
    expect(submit().attributes('disabled')).toBeDefined()

    await inputs[0]?.setValue(' 来週の火曜 ')
    expect(submit().attributes('disabled')).toBeDefined()
    await inputs[1]?.setValue('   ')
    expect(submit().attributes('disabled')).toBeDefined()
    await inputs[1]?.setValue('3 万円')
    expect(submit().attributes('disabled')).toBeUndefined()

    await submit().trigger('click')
    await flushPromises()
    expect(callsOf('AnswerAgentQuestions')[0]?.args).toEqual(['q1', ['来週の火曜', '3 万円']])
    // 返った 1 件（確認待ち）に行が変わる
    expect(w.find('[data-testid=agent-answer]').exists()).toBe(false)
    expect(card(w, 'q1').find('[data-testid=agent-open-draft]').exists()).toBe(true)
    w.unmount()
  })

  test('「あとで」は DeferAgentItem を呼ぶ。処理中は行のボタンを無効にし、二重に押しても 1 回だけ', async () => {
    const { w } = await openPanel([agentItem('r1')])
    const gate = deferred()
    gates.DeferAgentItem = gate.p
    responses.DeferAgentItem = agentItem('r1', { status: 'later' })
    const defer = card(w, 'r1').get('[data-testid=agent-defer]')
    await defer.trigger('click')
    expect(card(w, 'r1').get('[data-testid=agent-defer]').attributes('disabled')).toBeDefined()
    expect(card(w, 'r1').get('[data-testid=agent-open-draft]').attributes('disabled')).toBeDefined()
    await defer.trigger('click')
    gate.resolve()
    await flushPromises()
    expect(callsOf('DeferAgentItem')).toHaveLength(1)
    expect(card(w, 'r1').find('[data-testid=agent-defer]').exists()).toBe(false) // later になった（もう「あとで」は出さない）
    w.unmount()
  })

  test('返信不要は「やっぱり返信する」、失敗は「やり直す」で ReconsiderAgentItem を呼ぶ', async () => {
    const { w } = await openPanel([
      agentItem('s1', { status: 'skipped', draftId: undefined }),
      agentItem('f1', { status: 'failed', draftId: undefined }),
    ])
    responses.ReconsiderAgentItem = (id: unknown) => agentItem(String(id), { status: 'ready' })
    const skipped = card(w, 's1').get('[data-testid=agent-reconsider]')
    const failed = card(w, 'f1').get('[data-testid=agent-reconsider]')
    expect(skipped.text()).toBe('やっぱり返信する')
    expect(failed.text()).toBe('やり直す')
    await skipped.trigger('click')
    await failed.trigger('click')
    await flushPromises()
    expect(callsOf('ReconsiderAgentItem').map((c) => c.args)).toEqual([['s1'], ['f1']])
    w.unmount()
  })

  test('「開いて確認」は返信の作成画面を下書きつきで開き、開けたらパネルを閉じて確認済みにする', async () => {
    const { w, ui } = await openPanel([
      agentItem('r1', { accountId: 'a2', threadId: 'th1', draftId: 'dr1' }),
    ])
    const compose = useComposeStore()
    const open = mock(async (_req: unknown) => true)
    compose.open = open as never
    responses.ConfirmAgentItem = agentItem('r1', { status: 'done' })
    await card(w, 'r1').get('[data-testid=agent-open-draft]').trigger('click')
    await flushPromises()

    expect(open.mock.calls[0]).toEqual([
      { mode: 'reply_all', ref: { accountId: 'a2', threadId: 'th1' }, draftId: 'dr1' },
    ])
    expect(useInboxStore().selectedRef).toEqual({ accountId: 'a2', threadId: 'th1' })
    expect(callsOf('ConfirmAgentItem')[0]?.args).toEqual(['r1'])
    expect(ui.agentOpen).toBe(false)
    w.unmount()
  })

  test('作成画面を開けなかったら、確認済みにせずパネルも閉じない', async () => {
    const { w, ui } = await openPanel([agentItem('r1')])
    useComposeStore().open = (async () => false) as never
    await card(w, 'r1').get('[data-testid=agent-open-draft]').trigger('click')
    await flushPromises()
    expect(callsOf('ConfirmAgentItem')).toHaveLength(0)
    expect(ui.agentOpen).toBe(true)
    expect(
      card(w, 'r1').get('[data-testid=agent-open-draft]').attributes('disabled'),
    ).toBeUndefined()
    w.unmount()
  })

  test('下書きが無いのに開こうとしたら、アラートで知らせて何も開かない', async () => {
    const { w, ui } = await openPanel([agentItem('r1', { draftId: undefined })])
    const open = mock(async (_req: unknown) => true)
    useComposeStore().open = open as never
    await card(w, 'r1').get('[data-testid=agent-open-draft]').trigger('click')
    await flushPromises()
    expect(open).not.toHaveBeenCalled()
    expect(ui.alerts.map((a) => a.message)).toEqual(['開ける下書きがありません。'])
    w.unmount()
  })

  test('Esc と閉じるボタンでパネルを閉じる', async () => {
    const { w, ui } = await openPanel([], false)
    await w.get('aside').trigger('keydown', { key: 'Escape' })
    expect(ui.agentOpen).toBe(false)
    ui.openAgent()
    await w.get('[data-testid=agent-close]').trigger('click')
    expect(ui.agentOpen).toBe(false)
    w.unmount()
  })
})
