import { beforeEach, describe, expect, test } from 'bun:test'
import type { AIModelList } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import BaseSelect from '../../../ui/BaseSelect.vue'
import AIModelField from '../AIModelField.vue'
import {
  acctA,
  base,
  button,
  chatgptCfg,
  consentEl,
  flushPromises,
  type ModelsFn,
  mountAI,
  setup,
} from '../aiTestSupport'

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

const providerSelect = (w: ReturnType<typeof mountAI>) =>
  w.findAllComponents(BaseSelect).find((c) => c.props('ariaLabel') === 'プロバイダ')
const modelOptions = (w: ReturnType<typeof mountAI>) =>
  (
    w.getComponent(AIModelField).findComponent(BaseSelect).props('options') as { value: string }[]
  ).map((o) => o.value)

describe('プロバイダを選ぶ → 同意 → モデル一覧（#105）', () => {
  test('同意していないプロバイダを選ぶと同意の画面（送信先つき）が出て、同意すると保存し直さずに一覧が出る', async () => {
    const be = backend({ keyed: ['anthropic'] })
    const { pinia, store, loads } = setup(
      { provider: 'openai', model: 'gpt-old', consentedProviders: ['openai'] },
      undefined,
      be.fn,
    )
    const agreed: string[][] = []
    store.recordAIConsent = async (p: string, url = '') => {
      agreed.push([p, url])
      be.consented.add(`${p}|${url}`)
    }
    let configSaved = false
    store.setAIConfig = async () => {
      configSaved = true
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()

    // 1 回目の取得は、同意前なので Go が送らず consent_required を返す。画面は同意の画面を出す。
    expect(loads).toEqual([{ provider: 'anthropic', baseUrl: '' }])
    expect(consentEl('ai-consent-body')?.textContent).toContain(
      'Anthropic（https://api.anthropic.com）',
    )
    expect(agreed).toEqual([])

    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual([['anthropic', '']])
    expect(loads).toHaveLength(2)
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(modelOptions(w)).toContain('model-a')
    expect(configSaved).toBe(false) // 保存し直さない
    expect(providerSelect(w)?.props('modelValue')).toBe('anthropic') // 同意で下書きのプロバイダが戻らない
  })

  test('一覧から選んだモデルを、そのまま保存できる', async () => {
    const be = backend({ consented: ['anthropic|'], keyed: ['anthropic'] })
    const { pinia, store } = setup({ provider: 'openai', model: 'gpt-old' }, undefined, be.fn)
    const saved: string[][] = []
    store.setAIConfig = async (p: string, m: string, url: string) => {
      saved.push([p, m, url])
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    w.getComponent(AIModelField).findComponent(BaseSelect).vm.$emit('update:modelValue', 'model-b')
    await flushPromises()
    await button(w, '設定を保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['anthropic', 'model-b', '']])
  })

  test('同意済みのプロバイダを選ぶと、同意の画面を出さずにすぐ一覧が出る', async () => {
    const be = backend({ consented: ['anthropic|'], keyed: ['anthropic'] })
    const { pinia, store, loads } = setup(
      { provider: 'openai', model: 'gpt-old' },
      undefined,
      be.fn,
    )
    let recorded = false
    store.recordAIConsent = async () => {
      recorded = true
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    expect(loads).toEqual([{ provider: 'anthropic', baseUrl: '' }])
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(recorded).toBe(false)
    expect(modelOptions(w)).toEqual(['gpt-old', 'model-a', 'model-b']) // 保存済みのモデルは一覧に無くても残す
  })

  test('同意しなかった（閉じた）ときは取得し直さず、プロバイダの選択だけが残る。モデル名は手入力できる', async () => {
    const be = backend()
    const { pinia, store, loads } = setup({ provider: 'openai', model: '' }, undefined, be.fn)
    let recorded = false
    store.recordAIConsent = async () => {
      recorded = true
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    consentEl('ai-consent-cancel')?.click()
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(recorded).toBe(false)
    expect(loads).toHaveLength(1) // 同意前の 1 回だけ（Go は送信先へ何も送らない）。同意しないので取得し直さない
    expect(providerSelect(w)?.props('modelValue')).toBe('anthropic')
    const input = w.get('[data-testid="ai-model"] input')
    await input.setValue('claude-manual')
    expect((w.get('[data-testid="ai-model"] input').element as HTMLInputElement).value).toBe(
      'claude-manual',
    )
    // 設定画面の同意のボタンから、あとで同意し直せる。
    expect(w.find('[data-testid="ai-consent-now"]').exists()).toBe(true)
  })

  test('ChatGPT プラン: 未ログインなら、同意のあとログイン（Continue with ChatGPT）へ進み、ログイン後に一覧を取得する', async () => {
    const be = backend({ keyed: [] })
    const { pinia, store, loads } = setup({ provider: 'anthropic', model: 'm' }, undefined, be.fn)
    const events: string[] = []
    store.recordAIConsent = async (p: string, url = '') => {
      events.push(`consent:${p}`)
      be.consented.add(`${p}|${url}`)
    }
    store.signInChatGPT = async (id: string) => {
      events.push(`login:${id}`)
      be.keyed.add('chatgpt')
      store.aiConfig = { ...base, ...chatgptCfg([acctA]), consentedProviders: ['chatgpt'] }
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'chatgpt')
    await flushPromises()
    expect(consentEl('ai-consent-body')?.textContent).toContain('https://api.openai.com')
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(events).toEqual(['consent:chatgpt', 'login:'])
    expect(loads.map((l) => l.provider)).toEqual(['chatgpt', 'chatgpt', 'chatgpt'])
    expect(modelOptions(w)).toContain('model-a')
  })

  test('API キーのプロバイダ: 未登録ならキーの入力を促し、登録後に一覧を取得する（保存前のプロバイダへ登録できる）', async () => {
    const be = backend({ consented: ['anthropic|'], keyed: [] })
    const { pinia, store, loads } = setup({ provider: 'openai', model: 'm' }, undefined, be.fn)
    const keys: string[][] = []
    store.setAIApiKey = async (p: string, k: string) => {
      keys.push([p, k])
      be.keyed.add(p)
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    expect(w.get('[data-testid="ai-models-note"]').text()).toContain('API キーを登録すると')
    const key = w.get('input[type="password"]')
    expect(key.attributes('disabled')).toBeUndefined()
    await key.setValue('sk-dummy-value')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(keys).toEqual([['anthropic', 'sk-dummy-value']])
    expect(loads.map((l) => l.provider)).toEqual(['anthropic', 'anthropic'])
    expect(modelOptions(w)).toContain('model-a')
  })

  test('OpenAI 互換: Base URL が決まってから（Enter）取得し、その送信先で同意を取る', async () => {
    const url = 'https://llm.example/v1'
    const be = backend({ keyed: ['openai'] })
    const { pinia, store, loads } = setup({ provider: 'anthropic', model: 'm' }, undefined, be.fn)
    const agreed: string[][] = []
    store.recordAIConsent = async (p: string, u = '') => {
      agreed.push([p, u])
      be.consented.add(`${p}|${u}`)
    }
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'openai')
    await flushPromises()
    expect(loads).toEqual([]) // Base URL が決まるまで、既定の送信先へも同意を求めない
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(w.text()).toContain('Base URL を入力して Enter')

    const input = w.get('input[type="url"]')
    await input.setValue(url)
    await input.trigger('keydown.enter')
    await flushPromises()
    expect(loads).toEqual([{ provider: 'openai', baseUrl: url }])
    expect(consentEl('ai-consent-body')?.textContent).toContain(`OpenAI 互換（${url}）`)
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual([['openai', url]])
    expect(modelOptions(w)).toContain('model-a')
  })

  test.each([
    ['unauthorized', '認証または権限がないため、モデル一覧を取得できません。'],
    ['unsupported', 'このサービスにはモデル一覧の取得機能がないようです。'],
  ])('取得できないとき（%s）は注記を出して手入力にする', async (reason, message) => {
    const { pinia } = setup({ provider: 'openai', model: '' }, undefined, (p) => ({
      provider: p,
      models: [],
      reason,
      message,
    }))
    const w = mountAI(pinia)
    await providerSelect(w)?.vm.$emit('update:modelValue', 'anthropic')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(w.get('[data-testid="ai-models-note"]').text()).toContain(message)
    expect(w.find('[data-testid="ai-model"] input').exists()).toBe(true)
  })
})
