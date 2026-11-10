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

  test('既定のプロバイダがチェック（aria-checked）で選択中になり、補足に「設定済み」・「ログイン」・「API キー」を出す', () => {
    const { pinia } = setup({
      provider: 'openai',
      apiKeyProviders: ['openai', 'typesafe'],
      chatgptSignedIn: false,
    })
    const w = mountTab(pinia)
    expect(w.findAll('[data-testid="ai-provider-default"]')).toHaveLength(1)
    expect(card(w, 'openai').find('[data-testid="ai-provider-default"]').exists()).toBe(true)
    const note = (id: string) => w.get(`[data-testid="ai-provider-status-${id}"]`).text()
    expect(note('chatgpt')).toBe('ログイン')
    expect(note('anthropic')).toBe('API キー')
    expect(note('openai')).toBe('設定済み')
    expect(note('typesafe')).toBe('設定済み')
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

describe('プロバイダの一覧: カードのグリッド（#315）', () => {
  const grid = (w: VueWrapper) => w.get('[data-testid="ai-provider-grid"]')
  const cards = (w: VueWrapper) => w.findAll('[role="radio"]')
  const checked = (w: VueWrapper) => cards(w).map((c) => c.attributes('aria-checked'))
  const tabStops = (w: VueWrapper) => cards(w).map((c) => c.attributes('tabindex'))
  const focusedId = () => (document.activeElement as HTMLElement | null)?.dataset.testid
  const key = (w: VueWrapper, id: string, k: string) => card(w, id).trigger('keydown', { key: k })

  test('radiogroup の中に radio のカードが 4 枚（button）。サービス名・アイコン・補足を出し、title 属性は使わない', () => {
    const { pinia } = setup({ provider: 'anthropic' })
    const w = mountTab(pinia)
    expect(grid(w).attributes('role')).toBe('radiogroup')
    expect(grid(w).attributes('aria-label')).toBe('AI のプロバイダの一覧')
    expect(cards(w)).toHaveLength(4)
    for (const [id, name, initial] of [
      ['chatgpt', 'ChatGPT', 'C'],
      ['anthropic', 'Anthropic', 'A'],
      ['openai', 'OpenAI 互換', undefined],
      ['typesafe', 'TypeSafe AI（Jev）', 'T'],
    ] as const) {
      const c = card(w, id)
      expect(c.element.tagName).toBe('BUTTON')
      expect(c.attributes('role')).toBe('radio')
      expect(c.text()).toContain(name)
      const icon = c.get(`[data-provider-icon="${id}"]`)
      // 頭文字のタイル。OpenAI 互換だけは頭文字ではなく Iconoir のアイコン。
      if (initial) expect(icon.text()).toBe(initial)
      else expect(icon.find('svg').exists()).toBe(true)
    }
    expect(w.findAll('[title]')).toHaveLength(0)
    w.unmount()
  })

  test('選択中は既定のプロバイダの 1 枚だけ（aria-checked とチェック印）。既定が保存されていないときはどれも選択中にならない', () => {
    const { pinia } = setup({ provider: 'anthropic' })
    const w = mountTab(pinia)
    expect(checked(w)).toEqual(['false', 'true', 'false', 'false'])
    expect(card(w, 'anthropic').find('[data-testid="ai-provider-default"]').exists()).toBe(true)
    w.unmount()

    const none = setup({ provider: '' })
    const w2 = mountTab(none.pinia)
    expect(checked(w2)).toEqual(['false', 'false', 'false', 'false'])
    expect(w2.find('[data-testid="ai-provider-default"]').exists()).toBe(false)
    w2.unmount()
  })

  test('Tab で入れるのは 1 枚だけ（選択中があればそれ、無ければ先頭）', () => {
    const { pinia } = setup({ provider: 'openai' })
    const w = mountTab(pinia)
    expect(tabStops(w)).toEqual(['-1', '-1', '0', '-1'])
    w.unmount()

    const none = setup({ provider: '' })
    const w2 = mountTab(none.pinia)
    expect(tabStops(w2)).toEqual(['0', '-1', '-1', '-1'])
    w2.unmount()
  })

  test('矢印キーでフォーカスが次・前へ動き（端で回り込む）、Tab で入れるカードも付いてくる。選択（既定）は変わらない', async () => {
    const { pinia, store } = setup({ provider: 'anthropic' })
    const w = mountTab(pinia)
    const first = card(w, 'anthropic').element as HTMLElement
    first.focus()
    await key(w, 'anthropic', 'ArrowRight')
    expect(focusedId()).toBe('ai-provider-openai')
    await key(w, 'openai', 'ArrowDown')
    expect(focusedId()).toBe('ai-provider-typesafe')
    await key(w, 'typesafe', 'ArrowRight')
    expect(focusedId()).toBe('ai-provider-chatgpt')
    await key(w, 'chatgpt', 'ArrowLeft')
    expect(focusedId()).toBe('ai-provider-typesafe')
    await key(w, 'typesafe', 'ArrowUp')
    expect(focusedId()).toBe('ai-provider-openai')
    await key(w, 'openai', 'Home')
    expect(focusedId()).toBe('ai-provider-chatgpt')
    await key(w, 'chatgpt', 'End')
    expect(focusedId()).toBe('ai-provider-typesafe')

    expect(tabStops(w)).toEqual(['-1', '-1', '-1', '0'])
    expect(checked(w)).toEqual(['false', 'true', 'false', 'false'])
    expect(store.aiConfig?.provider).toBe('anthropic')
    expect(w.find('[data-testid="ai-provider-back"]').exists()).toBe(false)
    w.unmount()
  })

  test('カード全体を押す・フォーカスして押すとそのプロバイダのページが開く。矢印キーだけでは開かない', async () => {
    const { pinia, ui } = setup()
    const w = mountTab(pinia)
    await key(w, 'chatgpt', 'ArrowRight')
    expect(ui.aiProviderPage).toBeUndefined()
    await card(w, 'typesafe').trigger('click')
    expect(ui.aiProviderPage).toBe('typesafe')
    w.unmount()
  })
})
