import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { AIConfig } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import type { Capabilities } from '../../../bindings/github.com/ousiassllc/hitomado/internal/capability'
import { useAccountsStore } from '../../stores/accounts'
import { useNotificationsStore } from '../../stores/notifications'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import AISettings from './ai/AISettings.vue'
import { aiDestination } from './ai/aiDestination'
import GeneralSettings from './GeneralSettings.vue'
import McpSettings from './McpSettings.vue'
import OAuthClientsSettings from './oauth/OAuthClientsSettings.vue'
import SettingsDialog from './SettingsDialog.vue'
import { normalizeSignatureHtml } from './signatureHtml'

const info = {
  version: '1.2.3',
  updateCheckEnabled: true,
  autoUpdateSupported: false,
  autoUpdateEnabled: false,
  dataDir: '/data/Hitomado',
}

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

/** 保存済み（configured）の AI 設定。 */
function savedConfig(
  provider: string,
  model: string,
  hasApiKey: boolean,
  apiKeyProviders: string[],
  extra: Partial<AIConfig> = {},
): AIConfig {
  return {
    provider,
    model,
    configured: true,
    hasApiKey,
    apiKeyProviders,
    consentedProviders: [],
    chatgptSignedIn: false,
    chatgptAccounts: [],
    ...extra,
  }
}

describe('AISettings', () => {
  test('保存済みの送信先（プロバイダ・Base URL）を常に表示する', async () => {
    const pinia = setup()
    const settings = useSettingsStore()
    settings.aiConfig = savedConfig('openai', 'gpt-x', true, ['openai'], {
      baseUrl: 'https://llm.example.com/v1',
    })
    const w = mount(AISettings, { global: { plugins: [pinia] } })
    const dest = w.get('[data-testid="ai-destination"]').text()
    expect(dest).toContain('OpenAI 互換')
    expect(dest).toContain('https://llm.example.com/v1')

    settings.aiConfig = savedConfig('anthropic', 'm', false, [])
    await w.vm.$nextTick()
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://api.anthropic.com')
  })
})

describe('AISettings モデル名', () => {
  test('モデル名が空なら入力を促す注記を表示し、入力済みなら出さない', async () => {
    const pinia = setup()
    const settings = useSettingsStore()
    settings.aiConfig = savedConfig('anthropic', '', false, [])
    const w = mount(AISettings, { global: { plugins: [pinia] } })
    expect(w.text()).toContain('モデルを入力してください')

    settings.aiConfig = savedConfig('anthropic', 'm', false, [])
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

const ANDROID_CAPS: Capabilities = {
  os: 'android',
  supportsMcp: false,
  supportsTray: false,
  supportsAutoUpdate: false,
  supportsWindowState: false,
  singleInstance: false,
  touchUi: true,
}

describe('SettingsDialog（Android: 機能が無い OS）', () => {
  test('MCP を持たない OS では MCP のタブと説明の「MCP」を出さない', async () => {
    const pinia = setup()
    const ui = useUiStore()
    useSettingsStore().load = async () => {}
    usePlatformStore().capabilities = { ...ANDROID_CAPS }
    const w = mount(SettingsDialog, { global: { plugins: [pinia] }, attachTo: document.body })

    ui.openSettings()
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    const tabs = [...document.body.querySelectorAll('[role="tab"]')].map((t) =>
      t.textContent?.trim(),
    )
    expect(tabs).toEqual(['アカウント', 'OAuth クライアント', '署名', 'AI', 'ログ', '一般'])
    expect(document.body.querySelector('[role="dialog"]')?.textContent).not.toContain('MCP')
    w.unmount()
  })
})

describe('GeneralSettings', () => {
  test('アプリ内の更新確認がある OS では更新確認の項目を出す', () => {
    const pinia = setup()
    useSettingsStore().appInfo = info
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.text()).toContain('更新を確認する')
    expect(w.findAll('button').some((b) => b.text() === '今すぐ確認')).toBe(true)
    w.unmount()
  })

  test('更新確認を持たない OS（Android）では更新確認の項目を出さず、バージョンと保存先は出す', () => {
    const pinia = setup()
    useSettingsStore().appInfo = info
    usePlatformStore().capabilities = { ...ANDROID_CAPS }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.text()).not.toContain('更新を確認する')
    expect(w.findAll('button').some((b) => b.text() === '今すぐ確認')).toBe(false)
    expect(w.get('[data-testid="app-version"]').text()).toBe('1.2.3')
    expect(w.get('[data-testid="app-datadir"]').text()).toBe('/data/Hitomado')
    w.unmount()
  })
})

describe('GeneralSettings の通知の許可（Android）', () => {
  function mountWith(permission: 'granted' | 'denied' | 'unsupported') {
    const pinia = setup()
    useSettingsStore().appInfo = info
    const n = useNotificationsStore()
    // マウント時の読み直し（バインディング）は Go が無いテストでは失敗して unsupported になるため、読み直しを差し替える。
    n.refresh = async () => {}
    n.permission = permission
    const calls: string[] = []
    n.request = async () => {
      calls.push('request')
    }
    n.openSettings = async () => {
      calls.push('openSettings')
    }
    return { w: mount(GeneralSettings, { global: { plugins: [pinia] } }), calls }
  }

  test('許可を管理しない OS（Windows）では項目を出さない', () => {
    const { w } = mountWith('unsupported')
    expect(w.find('[data-testid="notification-permission"]').exists()).toBe(false)
    w.unmount()
  })

  test('許可済みなら状態だけを出し、ボタンは出さない', () => {
    const { w } = mountWith('granted')
    const box = w.get('[data-testid="notification-permission"]')
    expect(box.attributes('data-state')).toBe('granted')
    expect(box.text()).toContain('許可されています')
    expect(box.findAll('button')).toHaveLength(0)
    w.unmount()
  })

  test('拒否されていれば案内と、許可の要求・OS の設定を開くボタンを出す', async () => {
    const { w, calls } = mountWith('denied')
    const box = w.get('[data-testid="notification-permission"]')
    expect(box.attributes('data-state')).toBe('denied')
    expect(box.text()).toContain('許可されていません')
    const buttons = box.findAll('button')
    expect(buttons.map((b) => b.text())).toEqual(['通知を許可する', 'OS の通知設定を開く'])
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    expect(calls).toEqual(['request', 'openSettings'])
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
