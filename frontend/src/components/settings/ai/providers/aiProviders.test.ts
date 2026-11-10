import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import { base, button } from '../aiTestSupport'
import AIProviders from './AIProviders.vue'

beforeEach(() => {
  document.body.innerHTML = ''
})

function setup(cfg: Record<string, unknown> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.aiConfig = { ...base, ...cfg }
  const loads: string[] = []
  store.loadAIModels = async (p: string) => {
    loads.push(p)
    const r = { provider: p, models: [] }
    store.aiModels = { ...store.aiModels, [p]: r }
    return r
  }
  return { pinia, store, ui: useUiStore(), loads }
}

const mountTab = (pinia: ReturnType<typeof createPinia>) =>
  mount(AIProviders, { global: { plugins: [pinia] }, attachTo: document.body })

const card = (w: VueWrapper, id: string) => w.get(`[data-testid="ai-provider-${id}"]`)

describe('AI の設定: プロバイダの一覧（#315）', () => {
  test('最初はプロバイダの一覧。ChatGPT・Anthropic・OpenAI 互換・TypeSafe AI を、設定のフォームを出さずに並べる', () => {
    const { pinia } = setup()
    const w = mountTab(pinia)
    const ids = w
      .findAll('[data-testid^="ai-provider-"]')
      .map((b) => b.attributes('data-testid'))
      .filter((t) => /^ai-provider-(chatgpt|anthropic|openai|typesafe)$/.test(t ?? ''))
    expect(ids).toEqual([
      'ai-provider-chatgpt',
      'ai-provider-anthropic',
      'ai-provider-openai',
      'ai-provider-typesafe',
    ])
    expect(w.find('input').exists()).toBe(false)
    expect(w.find('[data-testid="ai-model"]').exists()).toBe(false)
    expect(card(w, 'typesafe').attributes('aria-label')).toBe('TypeSafe AI（Jev） の設定を開く')
    w.unmount()
  })

  test('既定のプロバイダに「既定」を付け、資格情報の状況（ログイン・API キー）を各行に出す', () => {
    const { pinia } = setup({
      provider: 'openai',
      apiKeyProviders: ['openai', 'typesafe'],
      chatgptSignedIn: false,
    })
    const w = mountTab(pinia)
    expect(w.findAll('[data-testid="ai-provider-default"]')).toHaveLength(1)
    expect(card(w, 'openai').find('[data-testid="ai-provider-default"]').exists()).toBe(true)
    const status = (id: string) => w.get(`[data-testid="ai-provider-status-${id}"]`).text()
    expect(status('chatgpt')).toBe('未ログイン')
    expect(status('anthropic')).toBe('API キー未設定')
    expect(status('openai')).toBe('API キー保存済み')
    expect(status('typesafe')).toBe('API キー保存済み')
    w.unmount()
  })

  test('押すとそのプロバイダの設定ページが開き、戻るで一覧に戻る。ページの間、一覧は出ない', async () => {
    const { pinia, ui } = setup()
    const w = mountTab(pinia)
    await card(w, 'anthropic').trigger('click')
    expect(ui.aiProviderPage).toBe('anthropic')
    expect(w.find('[data-testid="ai-provider-openai"]').exists()).toBe(false)
    expect(w.get('h3').text()).toBe('Anthropic')
    expect(w.find('[data-testid="ai-model"]').exists()).toBe(true)
    expect(w.find('input[type="password"]').exists()).toBe(true)

    await w.get('[data-testid="ai-provider-back"]').trigger('click')
    expect(ui.aiProviderPage).toBeUndefined()
    expect(w.find('[data-testid="ai-provider-openai"]').exists()).toBe(true)
    w.unmount()
  })

  test('保存済みの既定のプロバイダのページを開いただけでは取得しない。別のプロバイダのページは同意の確認・取得から始まり、既定になる前だと示す', async () => {
    const { pinia, loads } = setup({ provider: 'openai', consentedProviders: ['openai'] })
    const w = mountTab(pinia)
    await card(w, 'openai').trigger('click')
    await flushPromises()
    expect(loads).toEqual([])
    expect(w.find('[data-testid="ai-draft-hint"]').exists()).toBe(false)

    await w.get('[data-testid="ai-provider-back"]').trigger('click')
    await card(w, 'anthropic').trigger('click')
    await flushPromises()
    expect(loads).toEqual(['anthropic'])
    expect(w.get('[data-testid="ai-draft-hint"]').text()).toContain('既定')
    w.unmount()
  })

  test('別のプロバイダのページを開いても、保存済みの設定（既定のプロバイダ・モデル）は「設定を保存」まで変わらない', async () => {
    const { pinia, store } = setup({ provider: 'openai', model: 'gpt-old' })
    const saved: string[][] = []
    store.setAIConfig = async (p: string, m: string) => {
      saved.push([p, m])
    }
    const w = mountTab(pinia)
    await card(w, 'anthropic').trigger('click')
    await flushPromises()
    expect(store.aiConfig?.provider).toBe('openai')
    expect((w.get('[data-testid="ai-model"] input').element as HTMLInputElement).value).toBe('')
    await w.get('[data-testid="ai-model"] input').setValue('claude-x')
    await button(w, '設定を保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['anthropic', 'claude-x']])
    w.unmount()
  })

  test('初回の案内から ChatGPT へ進む入口（aiSetupIntent）は、ChatGPT のページを開いて流れを始める', async () => {
    const { pinia, ui } = setup({
      configured: false,
      provider: 'chatgpt',
      hasApiKey: false,
      apiKeyProviders: [],
      consentedProviders: [],
    })
    ui.openSettings('ai', 'chatgpt-login')
    const w = mountTab(pinia)
    await flushPromises()
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(ui.aiSetupIntent).toBeUndefined()
    expect(w.get('h3').text()).toBe('ChatGPT プラン（Continue with ChatGPT）')
    w.unmount()
  })
})

describe('TypeSafe AI（Jev）のページ（#314・#315）', () => {
  test('API キーだけを設定する。モデル・エフォート・Base URL・ログインの欄は出さず、送信先を表示する', async () => {
    const { pinia } = setup()
    const w = mountTab(pinia)
    await card(w, 'typesafe').trigger('click')
    expect(w.find('[data-testid="ai-typesafe-page"]').exists()).toBe(true)
    expect(w.find('[data-testid="ai-model"]').exists()).toBe(false)
    expect(w.find('input[type="url"]').exists()).toBe(false)
    expect(w.find('[data-testid="ai-chatgpt-login"]').exists()).toBe(false)
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://api.typesafe.ai')
    expect(w.text()).toContain('既定のプロバイダにはなりません')
    expect(w.find('input[type="password"]').exists()).toBe(true)
    w.unmount()
  })

  test('API キーを保存すると typesafe へ登録し、入力欄から消える。画面にキーを再表示しない。削除は確認を経由する', async () => {
    const { pinia, store, ui } = setup()
    const saved: string[][] = []
    const cleared: string[] = []
    store.setAIApiKey = async (p: string, k: string) => {
      saved.push([p, k])
      store.aiConfig = { ...base, apiKeyProviders: ['typesafe'] }
    }
    store.clearAIApiKey = async (p: string) => {
      cleared.push(p)
      store.aiConfig = { ...base, apiKeyProviders: [] }
    }
    const w = mountTab(pinia)
    await card(w, 'typesafe').trigger('click')
    expect(w.text()).toContain('未設定')

    await w.get('input[type="password"]').setValue('ts-secret-key')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['typesafe', 'ts-secret-key']])
    expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('ts-secret-key')
    expect(w.text()).toContain('保存済み')
    expect(ui.toast?.message).toContain('API キーを保存しました')

    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    expect(cleared).toEqual([])
    const confirm = [
      ...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button'),
    ].find((b) => b.textContent?.trim() === '削除する')
    confirm?.click()
    await flushPromises()
    expect(cleared).toEqual(['typesafe'])
    expect(w.text()).toContain('未設定')
    w.unmount()
  })

  test('保存に失敗したら理由を表示し、キーは入力欄に残さない扱いのまま（トーストを出さない）', async () => {
    const { pinia, store, ui } = setup()
    store.setAIApiKey = async () => {
      throw { code: 'INTERNAL', message: '資格情報ストアへ保存できません。', retryable: false }
    }
    const w = mountTab(pinia)
    await card(w, 'typesafe').trigger('click')
    await w.get('input[type="password"]').setValue('ts-secret-key')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('保存できません')
    expect(ui.toast).toBeUndefined()
    w.unmount()
  })
})
