import { beforeEach, describe, expect, test } from 'bun:test'
import type { AIModelList } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useUiStore } from '../../../../stores/ui'
import BaseSelect from '../../../ui/BaseSelect.vue'
import AIModelField from '../AIModelField.vue'
import { consentEl, flushPromises, type ModelsFn, mountAI, setup } from '../aiTestSupport'

beforeEach(() => {
  document.body.innerHTML = ''
})

const consentRequired = (provider: string): AIModelList => ({
  provider,
  models: [],
  reason: 'consent_required',
  message: '送信先への同意前は、モデル一覧を取得しません。モデル名を入力してください。',
})
const noCredentials = (provider: string): AIModelList => ({
  provider,
  models: [],
  reason: 'no_credentials',
  message: 'API キーを登録すると、使えるモデルを取得できます。モデル名を入力することもできます。',
})
const listed = (provider: string): AIModelList => ({
  provider,
  models: [{ id: 'model-a' }, { id: 'model-b', displayName: 'Model B' }],
})

/** Go と同じ決まり: 同意した送信先（provider|baseUrl）だけ一覧を返す。キーが無ければ no_credentials。 */
function backend(opts: { consented?: string[]; keyed?: string[] } = {}) {
  const consented = new Set(opts.consented ?? [])
  const keyed = new Set(opts.keyed ?? [])
  const fn: ModelsFn = (p, url) => {
    if (!consented.has(`${p}|${url}`)) return consentRequired(p)
    if (!keyed.has(p)) return noCredentials(p)
    return listed(p)
  }
  return { consented, keyed, fn }
}

const _modelOptions = (w: ReturnType<typeof mountAI>) =>
  (
    w.getComponent(AIModelField).findComponent(BaseSelect).props('options') as { value: string }[]
  ).map((o) => o.value)

describe('AI の既定のプロバイダ（#104）', () => {
  test('設定がまっさらなら、設定画面のプロバイダは ChatGPT プラン。取得は勝手にしない', async () => {
    const { pinia, calls } = setup({
      configured: false,
      provider: 'chatgpt',
      model: '',
      hasApiKey: false,
      apiKeyProviders: [],
      consentedProviders: [],
    })
    const w = mountAI(pinia)
    await flushPromises()
    expect(w.props('provider')).toBe('chatgpt')
    expect(w.find('[data-testid="ai-chatgpt-login"]').exists()).toBe(true)
    expect(calls).toEqual([])
  })

  test('設定がまだ読み込まれていないあいだも、既定は ChatGPT プラン', () => {
    const { pinia, store } = setup()
    store.aiConfig = undefined
    const w = mountAI(pinia)
    expect(w.props('provider')).toBe('chatgpt')
  })

  test.each(['anthropic', 'openai', 'chatgpt'])(
    '保存済みの %s は、既定が ChatGPT になっても変わらない',
    async (provider) => {
      const { pinia } = setup({ provider, model: 'saved-model', configured: true })
      const w = mountAI(pinia)
      await flushPromises()
      expect(w.props('provider')).toBe(provider)
      expect(w.find('[data-testid="ai-model"] input').element).toBeDefined()
    },
  )
})

describe('作成画面の初回の案内から ChatGPT へ進む流れ（#104・#105 と同じ部品）', () => {
  test('ChatGPT を選んだ状態で同意の画面 → 同意 → ログインまで自動で進む。意図は 1 回だけ使う', async () => {
    const be = backend({ keyed: [] })
    const { pinia, store, loads } = setup(
      {
        configured: false,
        provider: 'chatgpt',
        model: '',
        hasApiKey: false,
        consentedProviders: [],
      },
      undefined,
      be.fn,
    )
    const ui = useUiStore()
    ui.openSettings('ai', 'chatgpt-login')
    const events: string[] = []
    store.recordAIConsent = async (p: string, url = '') => {
      events.push(`consent:${p}`)
      be.consented.add(`${p}|${url}`)
    }
    store.signInChatGPT = async () => {
      events.push('login')
      be.keyed.add('chatgpt')
    }
    const w = mountAI(pinia)
    await flushPromises()
    expect(ui.aiSetupIntent).toBeUndefined()
    expect(w.props('provider')).toBe('chatgpt')
    expect(consentEl('ai-consent-body')?.textContent).toContain('https://api.openai.com')
    expect(events).toEqual([])
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(events).toEqual(['consent:chatgpt', 'login'])
    expect(loads.length).toBeGreaterThanOrEqual(2)
  })

  test('すでに同意済みなら、同意の画面を出さずにログインへ進む', async () => {
    const be = backend({ consented: ['chatgpt|'], keyed: [] })
    const { pinia, store } = setup(
      { configured: false, provider: 'chatgpt', model: '', hasApiKey: false },
      undefined,
      be.fn,
    )
    useUiStore().openSettings('ai', 'chatgpt-login')
    const events: string[] = []
    store.signInChatGPT = async () => {
      events.push('login')
    }
    mountAI(pinia)
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(events).toEqual(['login'])
  })

  test('意図が無ければ（設定画面を普通に開いたときは）取得もログインも始めない', async () => {
    const { pinia, store, calls } = setup({ configured: false, provider: 'chatgpt', model: '' })
    let logins = 0
    store.signInChatGPT = async () => {
      logins++
    }
    mountAI(pinia)
    await flushPromises()
    expect(calls).toEqual([])
    expect(logins).toBe(0)
  })
})
