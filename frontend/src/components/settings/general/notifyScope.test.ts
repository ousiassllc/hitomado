import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  callsOf,
  failures,
  installMocks,
  resetKit,
  restoreMocks,
} from '../../../stores/__tests__/settingsKit'

installMocks()
afterAll(restoreMocks)

// mock.module を適用した後で読み込む必要があるため、静的 import にできない。
const { useSettingsStore } = await import('../../../stores/settings')
const { default: GeneralSettings } = await import('../GeneralSettings.vue')

const info = {
  version: '1.2.3',
  updateCheckEnabled: true,
  autoUpdateSupported: false,
  autoUpdateEnabled: false,
  sendConfirmEnabled: true,
  notifyScope: 'primary',
  dataDir: '/data/Hitomado',
}

beforeEach(resetKit)

function setup(notifyScope: string | undefined) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.appInfo = { ...info, notifyScope: notifyScope as string }
  const w = mount(GeneralSettings, { global: { plugins: [pinia] } })
  const radio = (v: string) =>
    w.get<HTMLInputElement>(`[data-testid="notify-scope"] input[value="${v}"]`)
  return { w, radio, store }
}

describe('GeneralSettings の「通知するメール」（#395）', () => {
  test('保存値が「メインだけ」なら「メインだけ」を選んで表示する', () => {
    const { w, radio } = setup('primary')
    expect(radio('primary').element.checked).toBe(true)
    expect(radio('all').element.checked).toBe(false)
    w.unmount()
  })

  test('保存値が無い・未知の値のときも「メインだけ」として表示する', () => {
    for (const raw of [undefined, '', 'garbage']) {
      const { w, radio } = setup(raw)
      expect(radio('primary').element.checked).toBe(true)
      w.unmount()
    }
  })

  test('「すべて」を選ぶと保存して appInfo に反映する。「メインだけ」へも戻せる', async () => {
    const { w, radio, store } = setup('primary')
    await radio('all').setValue(true)
    await Promise.resolve()
    expect(callsOf('SetNotifyScope').map((c) => c.args)).toEqual([['all']])
    expect(store.appInfo?.notifyScope).toBe('all')
    expect(radio('all').element.checked).toBe(true)
    await radio('primary').setValue(true)
    await Promise.resolve()
    expect(callsOf('SetNotifyScope').map((c) => c.args)).toEqual([['all'], ['primary']])
    expect(store.appInfo?.notifyScope).toBe('primary')
    w.unmount()
  })

  test('保存に失敗したら appInfo を変えず、選択を保存済みの値に戻す', async () => {
    failures.SetNotifyScope = { message: JSON.stringify({ code: 'INTERNAL', message: 'x' }) }
    const { w, radio, store } = setup('primary')
    await radio('all').setValue(true)
    await new Promise((r) => setTimeout(r, 0))
    expect(store.appInfo?.notifyScope).toBe('primary')
    expect(radio('primary').element.checked).toBe(true)
    expect(radio('all').element.checked).toBe(false)
    w.unmount()
  })
})
