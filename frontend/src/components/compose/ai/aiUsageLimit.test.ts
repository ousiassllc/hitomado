import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { isUsageLimit, useAiDraft } from '../../../composables/useAiDraft'
import { useSettingsStore } from '../../../stores/settings'
import AiDraftButton from './AiDraftButton.vue'
import UsageLimitNotice from './UsageLimitNotice.vue'

beforeEach(() => {
  setActivePinia(createPinia())
})

const usageLimitError = {
  code: 'AI_FAILED',
  message: 'ChatGPT プランの利用枠に達しました。',
  retryable: false,
  details: { reason: 'usage_limit_exceeded' },
}

function drafter(generate: () => Promise<{ bodyHtml: string }>) {
  return useAiDraft({
    config: () => ({
      provider: 'chatgpt',
      configured: true,
      hasApiKey: true,
      consentedProviders: ['chatgpt'],
    }),
    hasBody: () => false,
    recordConsent: async () => {},
    generate,
    cancel: async () => {},
    openSetup: () => {},
    onGenerated: () => {},
    newRequestId: () => 'r1',
  })
}

describe('利用枠の超過（#80）', () => {
  test('details.reason だけで判定する（メッセージの文言や別の AI_FAILED では立てない）', () => {
    expect(isUsageLimit(usageLimitError as never)).toBe(true)
    expect(isUsageLimit({ ...usageLimitError, details: undefined } as never)).toBe(false)
    expect(isUsageLimit({ ...usageLimitError, details: { reason: 'other' } } as never)).toBe(false)
    expect(isUsageLimit({ ...usageLimitError, code: 'RATE_LIMITED' } as never)).toBe(false)
  })

  test('超過のときは error ではなく usageLimit を立て、次の生成の開始で下ろす。ほかの失敗は error のまま', async () => {
    let fail: unknown = usageLimitError
    const ai = drafter(async () => {
      throw fail
    })
    await ai.start()
    expect(ai.usageLimit.value).toBe(true)
    expect(ai.error.value).toBeUndefined()

    fail = { code: 'AI_FAILED', message: 'AI の生成に失敗しました。', retryable: false }
    await ai.start()
    expect(ai.usageLimit.value).toBe(false)
    expect(ai.error.value).toBe('AI の生成に失敗しました。')
  })

  test('案内は Manage usage が主なアクションで、閉じるが副。ChatGPT の見出しを残す', async () => {
    const w = mount(UsageLimitNotice)
    expect(w.text()).toContain('ChatGPT')
    expect(w.text()).toContain('Usage limit reached')
    expect(w.text()).toContain('Review your ChatGPT plan or app limit in ChatGPT settings.')
    const manage = w.get('[data-testid="ai-usage-limit-manage"]')
    expect(manage.text()).toBe('Manage usage')
    expect(manage.classes()).toContain('btn-primary')
    expect(w.get('[data-testid="ai-usage-limit-dismiss"]').classes()).not.toContain('btn-primary')
    expect(w.text()).not.toContain('credit')
    await manage.trigger('click')
    await w.get('[data-testid="ai-usage-limit-dismiss"]').trigger('click')
    expect(w.emitted('manage')).toHaveLength(1)
    expect(w.emitted('dismiss')).toHaveLength(1)
  })

  test('案内の見出しは作成画面の h2 の下なので h3 にする（h4 で h3 を飛ばさない）', () => {
    const w = mount(UsageLimitNotice)
    expect(w.find('h3.title').exists()).toBe(true)
    expect(w.find('h4').exists()).toBe(false)
  })

  test('AiDraftButton: 超過すると案内が出て、Manage usage で使用状況の画面を開く（失敗メッセージだけにしない）', async () => {
    const store = useSettingsStore()
    store.aiConfig = {
      provider: 'chatgpt',
      model: 'gpt-plan',
      configured: true,
      hasApiKey: true,
      apiKeyProviders: [],
      consentedProviders: ['chatgpt'],
      chatgptSignedIn: true,
      chatgptAccounts: [],
    }
    const opened: string[] = []
    store.openExternal = async (url: string) => {
      opened.push(url)
    }
    store.generateReplyDraft = () => Promise.reject(usageLimitError)
    const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="ai-usage-limit"]').exists()).toBe(true)
    expect(w.find('.ai-error').exists()).toBe(false)
    await w.get('[data-testid="ai-usage-limit-manage"]').trigger('click')
    expect(opened).toEqual(['https://chatgpt.com/settings/usage'])
    await w.get('[data-testid="ai-usage-limit-dismiss"]').trigger('click')
    expect(w.find('[data-testid="ai-usage-limit"]').exists()).toBe(false)
    w.unmount()
  })

  test('AiDraftButton: 生成中にアンマウント（作成セッションを閉じる）すると CancelGenerate を 1 回呼び、後から届いた結果は挿入しない。生成中でなければ呼ばない', async () => {
    const store = useSettingsStore()
    store.aiConfig = {
      provider: 'chatgpt',
      model: 'gpt-plan',
      configured: true,
      hasApiKey: true,
      apiKeyProviders: [],
      consentedProviders: ['chatgpt'],
      chatgptSignedIn: true,
      chatgptAccounts: [],
    }
    const cancelled: string[] = []
    store.cancelGenerate = async (id: string) => {
      cancelled.push(id)
    }
    const gen = Promise.withResolvers<{ bodyHtml: string }>()
    store.generateReplyDraft = () => gen.promise as never

    const idle = mount(AiDraftButton, { props: { sessionId: 's0' }, attachTo: document.body })
    idle.unmount()
    expect(cancelled).toEqual([])

    const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
    await w.get('button').trigger('click')
    await flushPromises()
    w.unmount()
    expect(cancelled).toHaveLength(1)
    gen.resolve({ bodyHtml: '<p>late</p>' })
    await flushPromises()
    expect(w.emitted('generated')).toBeUndefined()
    expect(cancelled).toHaveLength(1)
  })
})
