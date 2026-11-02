import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAccountsStore } from '../../stores/accounts'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import AISettings from './AISettings.vue'
import { aiDestination } from './aiDestination'
import OAuthClientsSettings from './OAuthClientsSettings.vue'
import SettingsDialog from './SettingsDialog.vue'
import { normalizeSignatureHtml } from './signatureHtml'

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return pinia
}

describe('OAuthClientsSettings', () => {
  test('使用中のクライアントは削除ボタンが無効で、使用中の件数を表示する。未使用は有効', () => {
    const pinia = setup()
    const accounts = useAccountsStore()
    accounts.oauthClients = [
      { id: 'c1', name: '個人用', clientId: 'id-1', accountCount: 2, hasSecret: true },
      { id: 'c2', name: '予備', clientId: 'id-2', accountCount: 0, hasSecret: true },
    ]
    const w = mount(OAuthClientsSettings, { global: { plugins: [pinia] } })

    const inUse = w.get('[data-client-id="c1"] button.remove')
    expect(inUse.attributes('disabled')).toBeDefined()
    expect(w.get('[data-client-id="c1"]').text()).toContain('2 件')
    expect(w.get('[data-client-id="c1"]').text()).toContain('削除できません')

    const free = w.get('[data-client-id="c2"] button.remove')
    expect(free.attributes('disabled')).toBeUndefined()
  })
})

describe('AISettings', () => {
  test('保存済みの送信先（プロバイダ・Base URL）を常に表示する', async () => {
    const pinia = setup()
    const settings = useSettingsStore()
    settings.aiConfig = {
      provider: 'openai',
      model: 'gpt-x',
      baseUrl: 'https://llm.example.com/v1',
      hasApiKey: true,
      consentedProviders: [],
    }
    const w = mount(AISettings, { global: { plugins: [pinia] } })
    const dest = w.get('[data-testid="ai-destination"]').text()
    expect(dest).toContain('OpenAI 互換')
    expect(dest).toContain('https://llm.example.com/v1')

    settings.aiConfig = {
      provider: 'anthropic',
      model: 'm',
      hasApiKey: false,
      consentedProviders: [],
    }
    await w.vm.$nextTick()
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://api.anthropic.com')
  })
})

describe('AISettings モデル名', () => {
  test('モデル名が空なら入力を促す注記を表示し、入力済みなら出さない', async () => {
    const pinia = setup()
    const settings = useSettingsStore()
    settings.aiConfig = {
      provider: 'anthropic',
      model: '',
      hasApiKey: false,
      consentedProviders: [],
    }
    const w = mount(AISettings, { global: { plugins: [pinia] } })
    expect(w.text()).toContain('モデルを入力してください')

    settings.aiConfig = {
      provider: 'anthropic',
      model: 'm',
      hasApiKey: false,
      consentedProviders: [],
    }
    await w.vm.$nextTick()
    expect(w.text()).not.toContain('モデルを入力してください')
  })
})

describe('aiDestination', () => {
  test('openai は Base URL 未指定で既定、指定ありでその URL。anthropic は Base URL を無視する', () => {
    expect(aiDestination('openai', '').url).toBe('https://api.openai.com')
    expect(aiDestination('openai', ' https://x.example/v1 ')).toMatchObject({
      url: 'https://x.example/v1',
      custom: true,
    })
    expect(aiDestination('anthropic', 'https://ignored.example').url).toBe(
      'https://api.anthropic.com',
    )
  })
})

describe('normalizeSignatureHtml', () => {
  test('テキストが無い HTML は空文字、あれば原文のまま', () => {
    expect(normalizeSignatureHtml('<p></p>')).toBe('')
    expect(normalizeSignatureHtml('<p><br></p><p>&nbsp;</p>')).toBe('')
    expect(normalizeSignatureHtml('<p>山田 <strong>太郎</strong></p>')).toBe(
      '<p>山田 <strong>太郎</strong></p>',
    )
  })
})

describe('SettingsDialog', () => {
  test('uiStore.dialogs.settings で開き、5 つのタブを出す', async () => {
    const pinia = setup()
    const ui = useUiStore()
    // 開いた時の load() は Wails ランタイムを呼ぶので、読み込み済みにして空振りさせる
    useSettingsStore().load = async () => {}
    const w = mount(SettingsDialog, { global: { plugins: [pinia] }, attachTo: document.body })
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()

    ui.openSettings()
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    const tabs = [...document.body.querySelectorAll('[role="tab"]')].map((t) =>
      t.textContent?.trim(),
    )
    expect(tabs).toEqual(['アカウント', 'OAuth クライアント', '署名', 'AI', '一般'])
    w.unmount()
  })
})
