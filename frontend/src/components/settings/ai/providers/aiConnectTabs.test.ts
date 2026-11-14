import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { createPinia } from 'pinia'
import { features } from '../../../../../test/aiConfigKit'
import { useUiStore } from '../../../../stores/ui'
import {
  acctA,
  base,
  button,
  chatgptCfg,
  consentEl,
  dialogButton,
  setup,
  until,
} from '../aiTestSupport'
import AIProviders from './AIProviders.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

const mountTab = (pinia: ReturnType<typeof createPinia>) =>
  mount(AIProviders, { global: { plugins: [pinia] }, attachTo: document.body })
const row = (w: VueWrapper, id: string) => w.get(`[data-testid="ai-provider-${id}"]`)
const chip = (w: VueWrapper, id: string) => w.get(`[data-testid="ai-provider-status-${id}"]`).text()

describe('AI の設定のタブ（#376）', () => {
  test('「AI のつなぎ先」と「機能ごとに使う AI」の 2 つのタブ。つないであれば後者から、何もなければ前者から開く', async () => {
    const { pinia } = setup()
    const w = mountTab(pinia)
    await flushPromises()
    expect(w.findAll('[role="tab"]').map((t) => t.text())).toEqual([
      'AI のつなぎ先',
      '機能ごとに使う AI',
    ])
    expect(w.find('[data-testid="ai-use-tab"]').exists()).toBe(true)
    expect(w.find('[data-testid="ai-connect-tab"]').exists()).toBe(false)
    w.unmount()

    document.body.innerHTML = ''
    const none = setup({ apiKeyProviders: [], consentedProviders: [], features: features() })
    const w2 = mountTab(none.pinia)
    await flushPromises()
    expect(w2.find('[data-testid="ai-connect-tab"]').exists()).toBe(true)
    w2.unmount()
  })

  test('タブを切り替えられる。選んだタブは覚える（ui.aiTab）', async () => {
    const { pinia } = setup()
    const ui = useUiStore()
    const w = mountTab(pinia)
    await flushPromises()
    await w.get('[data-testid="ai-tab-connect"]').trigger('mousedown', { button: 0 })
    await flushPromises()
    expect(w.find('[data-testid="ai-connect-tab"]').exists()).toBe(true)
    expect(ui.aiTab).toBe('connect')
    await w.get('[data-testid="ai-tab-use"]').trigger('mousedown', { button: 0 })
    await flushPromises()
    expect(w.find('[data-testid="ai-use-tab"]').exists()).toBe(true)
    expect(ui.aiTab).toBe('use')
    w.unmount()
  })
})

describe('「AI のつなぎ先」タブ（#376）', () => {
  function open(cfg = {}) {
    const s = setup(cfg)
    useUiStore().aiTab = 'connect'
    const w = mountTab(s.pinia)
    return { ...s, w }
  }

  test('プロバイダごとに 1 行（ChatGPT・Anthropic・OpenAI 互換・TypeSafe AI）。モデルの選択は出さない', () => {
    const { w } = open()
    const rows = w.findAll('[data-testid^="ai-provider-"][data-state]')
    expect(rows.map((r) => r.attributes('data-testid'))).toEqual([
      'ai-provider-chatgpt',
      'ai-provider-anthropic',
      'ai-provider-openai',
      'ai-provider-typesafe',
    ])
    expect(w.find('[role="combobox"]').exists()).toBe(false)
    expect(w.text()).toContain('Inbox Zero 専用')
    w.unmount()
  })

  test('状態チップは「接続済み」「未接続」「つなぎ直しが必要」。操作は ログイン／API キーを追加／接続を外す', () => {
    const { w } = open({
      ...chatgptCfg([{ ...acctA, signedIn: false }]), // ChatGPT: ログアウトした
      features: features({ provider: 'openai', model: 'm' }),
    })
    expect(chip(w, 'chatgpt')).toBe('つなぎ直しが必要')
    expect(row(w, 'chatgpt').get('[data-testid="ai-provider-connect-chatgpt"]').text()).toBe(
      'ログイン',
    )
    expect(chip(w, 'openai')).toBe('接続済み')
    expect(w.find('[data-testid="ai-provider-disconnect-openai"]').text()).toBe('接続を外す')
    expect(chip(w, 'anthropic')).toBe('未接続')
    expect(w.get('[data-testid="ai-provider-connect-anthropic"]').text()).toBe('API キーを追加')
    expect(w.text()).not.toContain('つないだ')
    w.unmount()
  })

  test('「ログイン」「API キーを追加」は、そのプロバイダのページを、つなぐ流れつきで開く。「設定」は流れを始めない', async () => {
    const { w } = open()
    const ui = useUiStore()
    await w.get('[data-testid="ai-provider-connect-anthropic"]').trigger('click')
    await flushPromises()
    expect(ui.aiProviderPage).toBe('anthropic')
    expect(consentEl('ai-consent-body')?.textContent).toContain('Anthropic') // 未同意なので、同意の画面から始まる
    w.unmount()

    document.body.innerHTML = ''
    const again = open()
    await again.w.get('[data-testid="ai-provider-open-openai"]').trigger('click')
    expect(useUiStore().aiProviderPage).toBe('openai')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    again.w.unmount()
  })

  test('資格情報だけで未同意なら、ボタンは「同意して接続」（同意の画面へ）', () => {
    const { w } = open({ consentedProviders: [] })
    expect(chip(w, 'openai')).not.toBe('接続済み')
    expect(w.get('[data-testid="ai-provider-connect-openai"]').text()).toBe('同意して接続')
    w.unmount()
  })

  test('「接続を外す」は確認を経由して API キーを削除する。選んでいた AI の選択は変えない', async () => {
    const { w, store } = open()
    const cleared: string[] = []
    store.clearAIApiKey = async (p: string) => {
      cleared.push(p)
      store.aiConfig = { ...store.aiConfig!, apiKeyProviders: [] }
    }
    await w.get('[data-testid="ai-provider-disconnect-openai"]').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain(
      '接続を外しますか？',
    )
    dialogButton('キャンセル')?.click()
    await flushPromises()
    expect(cleared).toEqual([])
    await w.get('[data-testid="ai-provider-disconnect-openai"]').trigger('click')
    await flushPromises()
    dialogButton('接続を外す')?.click()
    await flushPromises()
    expect(cleared).toEqual(['openai'])
    expect(chip(w, 'openai')).toBe('つなぎ直しが必要') // draft が openai を選んだまま
    expect(store.aiConfig?.features.draft.provider).toBe('openai')
    w.unmount()
  })

  test('ChatGPT の「接続を外す」は、ログイン中のアカウントをすべてログアウトする', async () => {
    const { w, store } = open({
      ...chatgptCfg([acctA, { ...acctA, id: 'oaiapp_BBBB', active: false }]),
      consentedProviders: ['chatgpt'],
    })
    const out: string[] = []
    store.signOutChatGPT = async (id: string) => {
      out.push(id)
      return true
    }
    await w.get('[data-testid="ai-provider-disconnect-chatgpt"]').trigger('click')
    await flushPromises()
    dialogButton('接続を外す')?.click()
    await flushPromises()
    expect(out).toEqual(['oaiapp_AAAA', 'oaiapp_BBBB'])
    w.unmount()
  })

  test('「AI で下書き」の初回の案内（aiSetupIntent）からは、ChatGPT のページをつなぐ流れつきで開く', async () => {
    const { pinia } = setup({ features: features(), apiKeyProviders: [], consentedProviders: [] })
    const ui = useUiStore()
    ui.openSettings('ai', 'chatgpt-login')
    const w = mountTab(pinia)
    await flushPromises()
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(ui.aiSetupIntent).toBeUndefined()
    await until(() => consentEl('ai-consent-body') !== null) // 未同意なので、同意の画面から
    w.unmount()
  })
})

describe('「AI で下書き」の初回の案内から（aiSetupIntent）', () => {
  test('すでに ChatGPT につないであれば、つなぎ先のページではなく「機能ごとに使う AI」を開く', async () => {
    const { pinia } = setup({
      ...chatgptCfg([acctA]),
      consentedProviders: ['chatgpt'],
      features: features(),
    })
    const ui = useUiStore()
    ui.openSettings('ai', 'chatgpt-login')
    const w = mountTab(pinia)
    await flushPromises()
    expect(ui.aiProviderPage).toBeUndefined()
    expect(w.find('[data-testid="ai-use-tab"]').exists()).toBe(true)
    w.unmount()
  })
})

describe('TypeSafe AI（Jev）のページ（#314・#315・#376）', () => {
  function open(cfg = {}) {
    const s = setup(cfg)
    useUiStore().aiTab = 'connect'
    const w = mountTab(s.pinia)
    return { ...s, w }
  }

  test('API キーと送信への同意だけ。モデル・エフォート・Base URL・ログインの欄は出さず、送信先を表示する', async () => {
    const { w } = open()
    await w.get('[data-testid="ai-provider-connect-typesafe"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="ai-typesafe-page"]').exists()).toBe(true)
    expect(w.find('[data-testid="ai-model"]').exists()).toBe(false)
    expect(w.find('input[type="url"]').exists()).toBe(false)
    expect(w.find('[data-testid="ai-chatgpt-login"]').exists()).toBe(false)
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://api.typesafe.ai')
    expect(w.text()).toContain('「AI で下書き」には選べず')
    expect(w.find('input[type="password"]').exists()).toBe(true)
    expect(consentEl('ai-consent-body')?.textContent).toContain('api.typesafe.ai') // つなぐ流れの同意の画面が先に出ている
    w.unmount()
  })

  test('API キーを保存すると typesafe へ登録し、入力欄から消える。画面にキーを再表示しない。削除は確認を経由する', async () => {
    const { w, store } = open({ consentedProviders: ['typesafe'] })
    const ui = useUiStore()
    const saved: string[][] = []
    const cleared: string[] = []
    store.setAIApiKey = async (p: string, k: string) => {
      saved.push([p, k])
      store.aiConfig = { ...base, apiKeyProviders: ['typesafe'], consentedProviders: ['typesafe'] }
    }
    store.clearAIApiKey = async (p: string) => {
      cleared.push(p)
      store.aiConfig = { ...base, apiKeyProviders: [], consentedProviders: ['typesafe'] }
    }
    ui.openAiProvider('typesafe', false)
    await flushPromises()
    expect(w.text()).toContain('未設定')

    await w.get('input[type="password"]').setValue('ts-secret-key')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['typesafe', 'ts-secret-key']])
    expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('ts-secret-key')
    expect(w.text()).toContain('保存済み')

    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    expect(cleared).toEqual([])
    dialogButton('削除する')?.click()
    await flushPromises()
    expect(cleared).toEqual(['typesafe'])
    w.unmount()
  })

  test('未同意なら同意を記録できる（Inbox Zero で送るのは件名・差出人・本文の先頭）', async () => {
    const { w, store } = open({ consentedProviders: [] })
    const agreed: string[] = []
    store.recordAIConsent = async (p: string) => {
      agreed.push(p)
    }
    useUiStore().openAiProvider('typesafe', false)
    await flushPromises()
    await w.get('[data-testid="ai-consent-now"]').trigger('click')
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual(['typesafe'])
    w.unmount()
  })

  test('戻るで「AI のつなぎ先」の一覧へ戻る', async () => {
    const { w } = open()
    useUiStore().openAiProvider('typesafe', false)
    await flushPromises()
    await w.get('[data-testid="ai-provider-back"]').trigger('click')
    expect(useUiStore().aiProviderPage).toBeUndefined()
    await flushPromises()
    expect(w.find('[data-testid="ai-connect-tab"]').exists()).toBe(true)
    w.unmount()
  })
})
