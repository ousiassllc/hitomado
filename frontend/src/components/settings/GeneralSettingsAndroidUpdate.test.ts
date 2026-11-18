import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { Capabilities } from '../../../bindings/github.com/ousiassllc/hitomado/internal/capability'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import GeneralSettings from './GeneralSettings.vue'

const info = {
  version: '1.2.3',
  updateCheckEnabled: true,
  autoUpdateSupported: false,
  autoUpdateEnabled: false,
  sendConfirmEnabled: true,
  notifyScope: 'primary',
  dataDir: '/data/Hitomado',
}

// Android: 更新確認（新しい版のお知らせとリンク）だけを持ち、自動アップデートは持たない。
const ANDROID_CAPS: Capabilities = {
  os: 'android',
  supportsMcp: false,
  supportsTray: false,
  supportsAutoUpdate: true,
  supportsWindowState: false,
  singleInstance: false,
  touchUi: true,
}

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return pinia
}

describe('GeneralSettings の更新（Android）', () => {
  test('更新確認と新しい版のリンクを出し、自動アップデートの項目は出さない（バージョンと保存先は出す）', () => {
    const pinia = setup()
    const settings = useSettingsStore()
    settings.appInfo = info
    settings.update = { available: true, version: '9.9.9', url: 'https://example.com/r' }
    usePlatformStore().capabilities = { ...ANDROID_CAPS }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.text()).toContain('更新を確認する')
    expect(w.text()).toContain('新しいバージョン 9.9.9 があります。')
    expect(w.get('a.link').attributes('href')).toBe('https://example.com/r')
    expect(w.find('[data-testid="auto-update"]').exists()).toBe(false)
    expect(w.get('[data-testid="app-version"]').text()).toBe('1.2.3')
    expect(w.get('[data-testid="app-datadir"]').text()).toBe('/data/Hitomado')
    w.unmount()
  })

  test('保存先のパスは / の直後で折り返せる（テキストは変えず、単語の途中では切らない。#400）', () => {
    const pinia = setup()
    useSettingsStore().appInfo = {
      ...info,
      dataDir: '/data/user/0/jp.ousias.hitomado/files/Hitomado',
    }
    usePlatformStore().capabilities = { ...ANDROID_CAPS }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    const dir = w.get('[data-testid="app-datadir"]')
    expect(dir.text()).toBe('/data/user/0/jp.ousias.hitomado/files/Hitomado')
    // 区切りごとに折り返しの候補（<wbr>）が 1 つずつ入る（/ が 6 つ、末尾の部分で 1 つ）
    expect(dir.findAll('wbr')).toHaveLength(7)
    w.unmount()
  })

  test('更新確認を持たない機能構成では更新確認の項目を出さない', () => {
    const pinia = setup()
    useSettingsStore().appInfo = info
    usePlatformStore().capabilities = { ...ANDROID_CAPS, supportsAutoUpdate: false }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.text()).not.toContain('更新を確認する')
    expect(w.findAll('button').some((b) => b.text() === '今すぐ確認')).toBe(false)
    w.unmount()
  })
})
