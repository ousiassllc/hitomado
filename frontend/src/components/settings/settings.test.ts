import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAccountsStore } from '../../stores/accounts'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import AISettings from './AISettings.vue'
import { aiDestination } from './aiDestination'
import McpSettings from './McpSettings.vue'
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
  test('uiStore.dialogs.settings で開き、7 つのタブを出す', async () => {
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
    expect(tabs).toEqual(['アカウント', 'OAuth クライアント', '署名', 'AI', 'MCP', 'ログ', '一般'])
    w.unmount()
  })
})

describe('McpSettings', () => {
  const status = {
    enabled: true,
    host: '127.0.0.1',
    port: 47650,
    allowNonLoopback: false,
    hasToken: false,
    listening: true,
    endpoint: 'http://127.0.0.1:47650/mcp',
  }

  test('状態・接続先・トークンの有無を表示し、発行直後だけトークンの値を見せる（閉じると消える）', async () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.loadMcp = async () => {}
    store.mcp = status
    store.issueMcpToken = async () => {
      store.mcp = { ...status, hasToken: true }
      return 'hmcp_shown_once'
    }
    const w = mount(McpSettings, { global: { plugins: [pinia] } })
    await w.vm.$nextTick()
    expect(w.get('[data-testid="mcp-state"]').text()).toBe('待ち受け中')
    expect(w.get('[data-testid="mcp-endpoint"]').text()).toBe('http://127.0.0.1:47650/mcp')
    expect(w.get('[data-testid="mcp-token-state"]').text()).toContain('未発行')
    expect(w.find('[data-testid="mcp-token-value"]').exists()).toBe(false)

    const issue = w.findAll('button').find((b) => b.text() === 'トークンを発行')
    await issue?.trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="mcp-token-value"]').text()).toBe('hmcp_shown_once')
    expect(w.get('[data-testid="mcp-token-state"]').text()).toContain('発行済み')
    expect(w.get('[data-testid="mcp-token-state"]').text()).not.toContain('hmcp_shown_once')
    w.unmount()
  })

  test('範囲外のポートは送らずにエラーを出す', async () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.loadMcp = async () => {}
    store.mcp = status
    let sent = 0
    store.setMcpConfig = async () => {
      sent++
    }
    const w = mount(McpSettings, { global: { plugins: [pinia] } })
    await w.vm.$nextTick()
    const port = w.findAll('input')[1]
    await port?.setValue('80')
    await w
      .findAll('button')
      .find((b) => b.text() === '設定を反映')
      ?.trigger('click')
    expect(sent).toBe(0)
    expect(w.text()).toContain('1024〜65535')
    w.unmount()
  })

  test('不正なポートが入っていても、オフにする操作は送れる（Go が保存済みの値を維持する）', async () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.loadMcp = async () => {}
    store.mcp = status
    const sent: boolean[] = []
    store.setMcpConfig = async (cfg) => {
      sent.push(cfg.enabled)
    }
    const w = mount(McpSettings, { global: { plugins: [pinia] } })
    await w.vm.$nextTick()
    await w.findAll('input')[1]?.setValue('80')
    await w.get('[role="switch"]').trigger('click')
    await flushPromises()
    expect(sent).toEqual([false])
    w.unmount()
  })
})
