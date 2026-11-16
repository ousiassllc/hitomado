import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { AIConfig } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { features } from '../../../../../test/aiConfigKit'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import AiDraftButton from '../../../compose/ai/AiDraftButton.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const el = (testid: string) => document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)

function mountButton(cfg: Partial<AIConfig>) {
  const store = useSettingsStore()
  store.aiConfig = {
    features: features(),
    apiKeyProviders: [],
    consentedProviders: [],
    chatgptSignedIn: false,
    chatgptAccounts: [],
    ...cfg,
  }
  const calls: string[] = []
  store.generateReplyDraft = async () => {
    calls.push('generate')
    return { bodyHtml: '<p>x</p>' }
  }
  store.recordAIConsent = async () => {
    calls.push('consent')
  }
  const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
  return { w, store, calls }
}

describe('「AI で下書き」: 「AI で下書き」に使う AI を選んでいないときの初回の案内（#104・#376）', () => {
  test('押すと、ChatGPT でログインする案内が出る。ボタンは無効にならず、自動では何も作らない', async () => {
    const { w, calls } = mountButton({})
    const btn = w.get('button')
    expect(btn.attributes('disabled')).toBeUndefined()
    expect(el('ai-setup')).toBeNull()
    await btn.trigger('click')
    await flushPromises()
    expect(el('ai-setup')?.textContent).toContain('ChatGPT')
    expect(el('ai-setup-chatgpt')?.textContent?.trim()).toBe('Continue with ChatGPT')
    expect(el('ai-setup-other')).not.toBeNull() // ほかのプロバイダ（API キー）を選ぶ道も残す
    expect(calls).toEqual([])
    w.unmount()
  })

  test('「Continue with ChatGPT」で、設定画面の AI の流れ（ChatGPT のログイン）へ進む', async () => {
    const { w, calls } = mountButton({})
    const ui = useUiStore()
    await w.get('button').trigger('click')
    el('ai-setup-chatgpt')?.click()
    await flushPromises()
    expect(ui.dialogs.settings).toBe(true)
    expect(ui.settingsTab).toBe('ai')
    expect(ui.aiSetupIntent).toBe('chatgpt-login')
    expect(el('ai-setup')).toBeNull()
    expect(calls).toEqual([]) // 下書きは「AI で下書き」をもう一度押したときだけ
    w.unmount()
  })

  test('「別のプロバイダを選ぶ」は AI の設定を開く（ChatGPT の流れは始めない）。「あとで」は閉じるだけ', async () => {
    const { w } = mountButton({})
    const ui = useUiStore()
    await w.get('button').trigger('click')
    el('ai-setup-other')?.click()
    await flushPromises()
    expect(ui.dialogs.settings).toBe(true)
    expect(ui.settingsTab).toBe('ai')
    expect(ui.aiSetupIntent).toBeUndefined()

    ui.closeDialog('settings')
    await w.get('button').trigger('click')
    el('ai-setup-later')?.click()
    await flushPromises()
    expect(el('ai-setup')).toBeNull()
    expect(ui.dialogs.settings).toBe(false)
    w.unmount()
  })

  test('保存済みの設定では案内を出さず、従来どおり（同意→生成）。同意は保存済みの送信先に記録する', async () => {
    const { w, calls, store } = mountButton({
      features: features({ provider: 'openai', model: 'gpt-x' }),
      baseUrl: 'https://llm.example/v1',
      apiKeyProviders: ['openai'],
    })
    const consents: string[][] = []
    store.recordAIConsent = async (p: string, url = '') => {
      consents.push([p, url])
      if (store.aiConfig) store.aiConfig = { ...store.aiConfig, consentedProviders: [p] }
    }
    await w.get('button').trigger('click')
    await flushPromises()
    expect(el('ai-setup')).toBeNull()
    expect(el('ai-consent-body')?.textContent).toContain('https://llm.example/v1')
    el('ai-consent-agree')?.click()
    await flushPromises()
    expect(consents).toEqual([['openai', 'https://llm.example/v1']])
    expect(calls).toEqual(['generate'])
    w.unmount()
  })

  test('保存済みでキー未設定は、従来どおりボタンを無効にして設定へ誘導する', async () => {
    const { w } = mountButton({ features: features({ provider: 'anthropic', model: 'm' }) })
    expect(w.get('button').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('API キーが未設定')
    w.unmount()
  })
})
