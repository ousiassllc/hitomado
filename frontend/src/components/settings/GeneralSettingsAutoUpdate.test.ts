import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useSettingsStore } from '../../stores/settings'
import GeneralSettings from './GeneralSettings.vue'

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return pinia
}

describe('GeneralSettings の自動アップデート（Windows 版）', () => {
  const info = {
    version: '1.2.3',
    updateCheckEnabled: true,
    autoUpdateSupported: true,
    autoUpdateEnabled: true,
    sendConfirmEnabled: true,
    dataDir: '/data/Hitomado',
  }

  test('入れ替えを行えない版では項目を出さない', () => {
    const pinia = setup()
    useSettingsStore().appInfo = { ...info, autoUpdateSupported: false, autoUpdateEnabled: false }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="auto-update"]').exists()).toBe(false)
    w.unmount()
  })

  test('入れ替え待ちなら「再起動して更新」を出し、押すと RestartToUpdate の経路を呼ぶ', async () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.appInfo = info
    store.updateStatus = { stage: 'ready', version: '1.3.0' }
    let restarted = 0
    store.restartToUpdate = async () => {
      restarted++
    }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.get('[data-testid="auto-update-status"]').text()).toContain('1.3.0')
    await w.get('[data-testid="restart-to-update"]').trigger('click')
    expect(restarted).toBe(1)
    w.unmount()
  })

  test('オフのときは「再起動して更新」も進みも出さない', () => {
    const pinia = setup()
    const store = useSettingsStore()
    store.appInfo = { ...info, autoUpdateEnabled: false }
    store.updateStatus = { stage: 'ready', version: '1.3.0' }
    const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
    expect(w.find('[data-testid="restart-to-update"]').exists()).toBe(false)
    expect(w.find('[data-testid="auto-update-status"]').exists()).toBe(false)
    w.unmount()
  })
})
