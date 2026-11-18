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
  const btn = (v: string) =>
    w.get<HTMLButtonElement>(`[data-testid="notify-scope"] button[data-value="${v}"]`)
  const pressed = (v: string) => btn(v).attributes('aria-pressed') === 'true'
  return { w, btn, pressed, store }
}

describe('GeneralSettings の「通知するメール」（#395）', () => {
  test('保存値が「メインだけ」なら「メインだけ」を押した状態で表示する', () => {
    const { w, pressed } = setup('primary')
    expect(pressed('primary')).toBe(true)
    expect(pressed('all')).toBe(false)
    w.unmount()
  })

  test('保存値が無い・未知の値のときも「メインだけ」として表示する', () => {
    for (const raw of [undefined, '', 'garbage']) {
      const { w, pressed } = setup(raw)
      expect(pressed('primary')).toBe(true)
      w.unmount()
    }
  })

  test('切り替えは 2 つのボタンが横に並び、説明は指定の文言で見出しのすぐ下に出す（「すべて」の説明は無い）', () => {
    const { w } = setup('primary')
    const root = w.get('[data-testid="notify-scope"]')
    const kids = Array.from(root.element.children).map((e) => e.tagName)
    expect(kids).toEqual(['STRONG', 'P', 'FIELDSET'])
    expect(root.get('strong').text()).toBe('通知するメール')
    expect(root.get('p').text()).toBe(
      'メインだけにすると、ソーシャル・プロモーションなどのタブのメールは通知しません。全アカウント共通です（アカウントごとのオン・オフは「アカウント」で設定）。',
    )
    expect(root.findAll('button').map((b) => b.text())).toEqual(['メインだけ', 'すべて'])
    expect(root.find('input').exists()).toBe(false)
    w.unmount()
  })

  test('「すべて」を押すと保存して appInfo に反映する。「メインだけ」へも戻せる', async () => {
    const { w, btn, pressed, store } = setup('primary')
    await btn('all').trigger('click')
    await Promise.resolve()
    expect(callsOf('SetNotifyScope').map((c) => c.args)).toEqual([['all']])
    expect(store.appInfo?.notifyScope).toBe('all')
    expect(pressed('all')).toBe(true)
    expect(pressed('primary')).toBe(false)
    await btn('primary').trigger('click')
    await Promise.resolve()
    expect(callsOf('SetNotifyScope').map((c) => c.args)).toEqual([['all'], ['primary']])
    expect(store.appInfo?.notifyScope).toBe('primary')
    w.unmount()
  })

  test('選択中のボタンをもう一度押しても保存しない', async () => {
    const { w, btn } = setup('primary')
    await btn('primary').trigger('click')
    expect(callsOf('SetNotifyScope')).toEqual([])
    w.unmount()
  })

  test('保存に失敗したら appInfo を変えず、表示を保存済みの値のままにする', async () => {
    failures.SetNotifyScope = { message: JSON.stringify({ code: 'INTERNAL', message: 'x' }) }
    const { w, btn, pressed, store } = setup('primary')
    await btn('all').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    expect(store.appInfo?.notifyScope).toBe('primary')
    expect(pressed('primary')).toBe(true)
    expect(pressed('all')).toBe(false)
    w.unmount()
  })
})
