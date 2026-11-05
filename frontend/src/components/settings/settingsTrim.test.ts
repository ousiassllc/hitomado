import { describe, expect, test } from 'bun:test'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { settings } from '../../lib/messages'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import AccountsSettings from './accounts/AccountsSettings.vue'
import GeneralSettings from './GeneralSettings.vue'
import LogSettings from './LogSettings.vue'
import McpSettings from './McpSettings.vue'
import OAuthClientsSettings from './oauth/OAuthClientsSettings.vue'
import SettingsDialog from './SettingsDialog.vue'
import SignatureSettings from './SignatureSettings.vue'

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return pinia
}

describe('設定ダイアログの冗長な表示を出さない', () => {
  test('見出し直下の説明文を出さず、aria-describedby も付けない', async () => {
    const pinia = setup()
    useSettingsStore().load = async () => {}
    const w = mount(SettingsDialog, { global: { plugins: [pinia] }, attachTo: document.body })
    useUiStore().openSettings()
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.querySelector('.dlg-desc')).toBeNull()
    expect(dialog?.getAttribute('aria-describedby')).toBeFalsy()
    w.unmount()
  })

  // タブ名と同じ見出し（h3）は出さない。タブの領域名は section の aria-label で残す。
  const tabs: [string, () => VueWrapper, string][] = [
    [
      'アカウント',
      () => mount(AccountsSettings, { global: { plugins: [setup()] } }),
      settings.tabs.accounts,
    ],
    [
      'OAuth クライアント',
      () => mount(OAuthClientsSettings, { global: { plugins: [setup()] } }),
      settings.tabs.oauthClients,
    ],
    [
      '署名',
      () => mount(SignatureSettings, { global: { plugins: [setup()] } }),
      settings.tabs.signature,
    ],
    ['ログ', () => mount(LogSettings, { global: { plugins: [setup()] } }), settings.tabs.logs],
    [
      '一般',
      () => mount(GeneralSettings, { global: { plugins: [setup()] } }),
      settings.tabs.general,
    ],
  ]
  for (const [name, render, label] of tabs) {
    test(`${name}タブは同名の見出しを出さず、領域名を aria-label に持つ`, () => {
      const w = render()
      expect(w.find('h3').exists()).toBe(false)
      expect(w.get('section').attributes('aria-label')).toBe(label)
      w.unmount()
    })
  }
})

describe('McpSettings の接続方法', () => {
  test('クライアントの設定方法は既定で閉じた折りたたみに置く', async () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.loadMcp = async () => {}
    store.mcp = {
      enabled: true,
      host: '127.0.0.1',
      port: 47650,
      allowNonLoopback: false,
      hasToken: false,
      listening: true,
      endpoint: 'http://127.0.0.1:47650/mcp',
    }
    const w = mount(McpSettings, { global: { plugins: [pinia] } })
    await w.vm.$nextTick()
    const details = w.get('details')
    expect(details.attributes('open')).toBeUndefined()
    expect(details.get('summary').text()).toBe(settings.mcp.connectHeading)
    expect(details.text()).toContain(settings.mcp.clientHint)
    // 折りたたみの外に同じ説明を二重に出さない。
    expect(w.text().split(settings.mcp.clientHint)).toHaveLength(2)
    expect(w.text()).toContain(settings.mcp.enableHint)
    w.unmount()
  })
})

describe('LogSettings の保存場所とローテーション', () => {
  test('記録しない情報は常に見え、保存場所と保存期間（5 MB・5 世代・30 日）は閉じた折りたたみに置く', () => {
    const w = mount(LogSettings, { global: { plugins: [setup()] } })
    const details = w.get('details')
    expect(details.attributes('open')).toBeUndefined()
    expect(details.get('summary').text()).toBe(settings.logs.storageHeading)
    expect(details.text()).toContain(settings.logs.intro)
    expect(settings.logs.intro).toContain('5 MB')
    expect(settings.logs.intro).toContain('5 世代')
    expect(settings.logs.intro).toContain('30 日')
    // 記録しない情報は折りたたみの外にあり、二重には出ない。
    expect(details.text()).not.toContain(settings.logs.redactHint)
    expect(w.text().split(settings.logs.redactHint)).toHaveLength(2)
    expect(w.text().split(settings.logs.intro)).toHaveLength(2)
    w.unmount()
  })
})
