import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  callsOf,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from '../../stores/__tests__/settingsKit'

installMocks()
afterAll(restoreMocks)

// mock.module を適用した後で読み込む必要があるため、静的 import にできない。
const { useSettingsStore } = await import('../../stores/settings')
const { default: GeneralSettings } = await import('./GeneralSettings.vue')

const info = {
  version: '1.2.3',
  updateCheckEnabled: true,
  autoUpdateSupported: false,
  autoUpdateEnabled: false,
  sendConfirmEnabled: true,
  dataDir: '/data/Hitomado',
}

beforeEach(() => {
  resetKit()
  responses.SetSendConfirmEnabled = undefined
})

function setup(sendConfirmEnabled: boolean) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.appInfo = { ...info, sendConfirmEnabled }
  const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
  const sw = () => w.get('[data-testid="send-confirm"] [role="switch"]')
  return { w, sw, store }
}

describe('GeneralSettings の送信前の確認画面（#313）', () => {
  test('オンのときスイッチはオン。切り替えると保存し、スイッチに反映する', async () => {
    const { w, sw, store } = setup(true)
    expect(sw().attributes('aria-checked')).toBe('true')
    await sw().trigger('click')
    await Promise.resolve()
    expect(callsOf('SetSendConfirmEnabled').map((c) => c.args)).toEqual([[false]])
    expect(store.appInfo?.sendConfirmEnabled).toBe(false)
    expect(sw().attributes('aria-checked')).toBe('false')
    await sw().trigger('click')
    await Promise.resolve()
    expect(callsOf('SetSendConfirmEnabled').map((c) => c.args)).toEqual([[false], [true]])
    w.unmount()
  })

  test('オフの保存値はオフで表示する', () => {
    const { w, sw } = setup(false)
    expect(sw().attributes('aria-checked')).toBe('false')
    w.unmount()
  })
})
