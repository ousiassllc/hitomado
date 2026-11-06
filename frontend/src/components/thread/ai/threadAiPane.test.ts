import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { appError, ctl, detail, REF, until } from '../threadTestKit'
import {
  aiCallsOf,
  aiCtl,
  installAiMocks,
  READY_CONFIG,
  resetAiState,
  restoreAiMocks,
  result,
} from './threadAiKit'

installAiMocks()
afterAll(restoreAiMocks)

const { useThreadStore } = await import('../../../stores/thread')
const { useSettingsStore } = await import('../../../stores/settings')
const { default: ThreadPane } = await import('../ThreadPane.vue')

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
  setActivePinia(createPinia())
  resetAiState()
  ctl.responses.GetThread = detail(false)
})

const tid = (id: string) => `[data-testid="${id}"]`
const body = (id: string) => document.body.querySelector<HTMLElement>(tid(id))

async function open(config: Record<string, unknown> = READY_CONFIG) {
  const settings = useSettingsStore()
  settings.aiConfig = config as never
  const w = mount(ThreadPane, { attachTo: document.body })
  await useThreadStore().open(REF)
  await flushPromises()
  return w
}

describe('ThreadPane の AI パネル（F-17・F-18）', () => {
  test('スレッドを開いただけ・新着の再取得では AI を呼ばない。質問欄だけが出ている', async () => {
    const w = await open()
    await useThreadStore().reload()
    await flushPromises()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(0)
    expect(aiCallsOf('AskAboutThread')).toHaveLength(0)
    expect(w.find(tid('thread-ai-panel')).exists()).toBe(true)
    expect(w.find(tid('thread-ai-question-input')).exists()).toBe(true)
    expect(w.find(tid('thread-ai-summary')).exists()).toBe(false)
    expect(w.find(tid('thread-ai-conversation')).exists()).toBe(false)
  })

  test('「AI で要約」を押したときだけ要約を生成し、AI パネルに表示する。再生成もできる', async () => {
    const w = await open()
    await w.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(1)
    expect(w.get(tid('thread-ai-summary-text')).text()).toBe('- 要約')

    aiCtl.responses.SummarizeThread = result('- 新しい要約')
    await w.get(tid('thread-ai-resummarize')).trigger('click')
    await flushPromises()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(2)
    expect(w.get(tid('thread-ai-summary-text')).text()).toBe('- 新しい要約')
  })

  test('長いスレッドで古いメッセージを外したときは、その旨を表示する', async () => {
    aiCtl.responses.SummarizeThread = result('- 要約', 3, 8)
    const w = await open()
    await w.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()
    expect(w.get(tid('thread-ai-truncated')).text()).toBe(
      '長いスレッドのため、新しい 3 件（全 8 件）を対象にしました',
    )
  })

  test('生成中は「AI で要約」を押せず、「中止」で戻る。失敗してもパネルの中に出し、直前の要約は残す', async () => {
    const w = await open()
    await w.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()

    let finish!: () => void
    aiCtl.responses.SummarizeThread = () =>
      new Promise(
        (_, reject) =>
          (finish = () => reject(appError('AI_FAILED', 'ネットワークに接続できません。'))),
      )
    await w.get(tid('thread-ai-resummarize')).trigger('click')
    await until(() => w.find(tid('thread-ai-cancel')).exists())
    expect(w.get(tid('act-ai-summarize')).attributes('disabled')).toBeDefined()
    expect(w.get(tid('thread-ai-question-input')).attributes('disabled')).toBeDefined()

    finish()
    await flushPromises()
    expect(w.get(tid('thread-ai-error')).text()).toContain('ネットワークに接続できません。')
    expect(w.get(tid('thread-ai-summary-text')).text()).toBe('- 要約')
  })

  test('質問は「質問する」ボタンで送り、質問と回答が会話に並ぶ。入力中は送らない', async () => {
    const w = await open()
    const input = w.get(tid('thread-ai-question-input'))
    await input.setValue('期限はいつ？')
    expect(aiCallsOf('AskAboutThread')).toHaveLength(0)
    await w.get(tid('thread-ai-ask')).trigger('click')
    await flushPromises()
    expect(aiCallsOf('AskAboutThread')).toHaveLength(1)
    const conv = w.get(tid('thread-ai-conversation')).text()
    expect(conv).toContain('あなたの質問')
    expect(conv).toContain('期限はいつ？')
    expect(conv).toContain('AI の回答')
    expect(conv).toContain('回答')
    expect((input.element as HTMLTextAreaElement).value).toBe('')
  })

  test('失敗した質問は会話に加わらず、入力欄に残って再送できる', async () => {
    const w = await open()
    const input = w.get(tid('thread-ai-question-input'))
    await input.setValue('期限は？')
    aiCtl.failures.AskAboutThread = appError('RATE_LIMITED', '混み合っています。')
    await w.get(tid('thread-ai-ask')).trigger('click')
    await flushPromises()
    expect(w.find(tid('thread-ai-conversation')).exists()).toBe(false)
    expect(w.get(tid('thread-ai-error')).text()).toContain('混み合っています。')
    expect((input.element as HTMLTextAreaElement).value).toBe('期限は？')

    aiCtl.failures = {}
    await w.get(tid('thread-ai-ask')).trigger('click')
    await flushPromises()
    expect(w.get(tid('thread-ai-conversation')).text()).toContain('期限は？')
  })

  test('「新しい会話」で会話だけが消え、要約は残る', async () => {
    const w = await open()
    await w.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()
    await w.get(tid('thread-ai-question-input')).setValue('q')
    await w.get(tid('thread-ai-ask')).trigger('click')
    await flushPromises()
    await w.get(tid('thread-ai-new-conversation')).trigger('click')
    expect(w.find(tid('thread-ai-conversation')).exists()).toBe(false)
    expect(w.find(tid('thread-ai-summary-text')).exists()).toBe(true)
  })

  test('別のスレッドを開くと、要約・会話・書きかけが画面から消える', async () => {
    const w = await open()
    await w.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()
    await w.get(tid('thread-ai-question-input')).setValue('書きかけ')
    await useThreadStore().open({ accountId: 'a1', threadId: 't2' })
    await flushPromises()
    expect(w.find(tid('thread-ai-summary')).exists()).toBe(false)
    expect((w.get(tid('thread-ai-question-input')).element as HTMLTextAreaElement).value).toBe('')
    expect(localStorage.length).toBe(0)
  })

  test('キー未設定では要約・質問を送れず、設定への案内を出す', async () => {
    const w = await open({ ...READY_CONFIG, hasApiKey: false })
    expect(w.get(tid('act-ai-summarize')).attributes('disabled')).toBeDefined()
    expect(w.get(tid('thread-ai-need-key')).text()).toContain('API キーが未設定')
    expect(w.get(tid('thread-ai-question-input')).attributes('disabled')).toBeDefined()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(0)
  })

  test('設定が未保存なら初回の案内、未同意なら同意の画面（F-08 と同じ）。同意したら押された操作を続ける', async () => {
    const first = await open({
      ...READY_CONFIG,
      configured: false,
      hasApiKey: false,
      consentedProviders: [],
    })
    await first.get(tid('act-ai-summarize')).trigger('click')
    await flushPromises()
    expect(body('ai-setup')).not.toBeNull()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(0)
    first.unmount()
    document.body.innerHTML = ''

    setActivePinia(createPinia())
    resetAiState()
    const w = await open({ ...READY_CONFIG, consentedProviders: [] })
    await w.get(tid('thread-ai-question-input')).setValue('期限は？')
    await w.get(tid('thread-ai-ask')).trigger('click')
    await flushPromises()
    expect(body('ai-consent-body')?.textContent).toContain(
      '要約・質問ではスレッドの全メッセージ（上限あり）が送信され',
    )
    expect(aiCallsOf('AskAboutThread')).toHaveLength(0)
    body('ai-consent-agree')?.click()
    await until(() => aiCallsOf('AskAboutThread').length === 1)
    expect(aiCallsOf('RecordAIConsent')).toHaveLength(1)
  })
})
