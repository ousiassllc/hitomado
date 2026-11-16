import { afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { setup } from '../../src/components/settings/ai/aiTestSupport'
import SettingsDialog from '../../src/components/settings/SettingsDialog.vue'
import { useLayoutStore } from '../../src/stores/layout'
import { usePlatformStore } from '../../src/stores/platform'
import { useSettingsStore } from '../../src/stores/settings'
import { useUiStore } from '../../src/stores/ui'

// 390 幅（1 画面ずつ）の設定ダイアログ: タブ行は横にスクロールし、選んでいるタブが見える位置に来る（#383）。
// happy-dom にはレイアウトが無いので、タブ行（幅 324）とタブ（1 つ 100 幅、順に並ぶ）の座標を差し込む。
const TAB_WIDTH = 100
const LIST_WIDTH = 324
const TAB_COUNT = 7 // MCP のタブがある OS（Windows・Linux・macOS）: accounts oauth signature ai mcp logs general

const proto = HTMLElement.prototype
const originals = (['offsetLeft', 'offsetWidth', 'clientWidth'] as const).map(
  (k) => [k, Object.getOwnPropertyDescriptor(proto, k)] as const,
)
function stubGeometry() {
  const tabIndex = (el: HTMLElement) =>
    el.classList.contains('tab') && el.parentElement
      ? Array.from(el.parentElement.children).indexOf(el)
      : 0
  Object.defineProperty(proto, 'offsetLeft', {
    configurable: true,
    get(this: HTMLElement) {
      return tabIndex(this) * TAB_WIDTH
    },
  })
  Object.defineProperty(proto, 'offsetWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.classList.contains('tab') ? TAB_WIDTH : 0
    },
  })
  Object.defineProperty(proto, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.classList.contains('tab-list') ? LIST_WIDTH : 0
    },
  })
}

const mediaOriginal = window.matchMedia
function startAt(width: number, touchUi: boolean) {
  window.matchMedia = ((q: string) => ({
    matches: width >= Number(/min-width: (\d+)px/.exec(q)?.[1]),
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia
  usePlatformStore().capabilities = {
    ...usePlatformStore().capabilities,
    touchUi,
    supportsMcp: true,
  }
  useLayoutStore().start()
}

beforeEach(stubGeometry)
afterEach(() => {
  for (const [k, d] of originals) {
    if (d) Object.defineProperty(proto, k, d)
    else delete (proto as unknown as Record<string, unknown>)[k]
  }
  useLayoutStore().dispose()
  window.matchMedia = mediaOriginal
  document.documentElement.removeAttribute('data-touch-ui')
  delete document.documentElement.dataset.layout
  document.body.innerHTML = ''
})

async function mountSettings(width: number, touchUi: boolean) {
  const { pinia } = setup()
  startAt(width, touchUi)
  useSettingsStore().load = async () => {} // 開いた時の load() は Wails ランタイムを呼ぶので空振りさせる
  const ui = useUiStore()
  const w = mount(SettingsDialog, { global: { plugins: [pinia] }, attachTo: document.body })
  const list = () => document.body.querySelector<HTMLElement>('.tab-list')
  return { w, ui, list }
}

describe('SettingsDialog のタブ行（390 幅）', () => {
  test('AI タブを選んで開くと、開いた直後から AI タブが見える位置までスクロールしている', async () => {
    const { w, ui, list } = await mountSettings(390, true)
    ui.openSettings('ai') // 4 番目（offsetLeft 300・右端 400）。窓は 0〜324
    await flushPromises()
    expect(list()?.scrollLeft).toBe(400 - LIST_WIDTH)
    w.unmount()
  })

  test('タブを切り替えると、選んだタブが見える位置に来る。見えているタブへの切り替えでは動かさない', async () => {
    const { w, ui, list } = await mountSettings(390, true)
    ui.openSettings('accounts')
    await flushPromises()
    expect(list()?.scrollLeft).toBe(0)
    ui.settingsTab = 'general' // 7 番目（offsetLeft 600・右端 700）
    await flushPromises()
    expect(list()?.scrollLeft).toBe(700 - LIST_WIDTH)
    ui.settingsTab = 'logs' // 6 番目（500〜600）はすでに窓（376〜700）の中
    await flushPromises()
    expect(list()?.scrollLeft).toBe(700 - LIST_WIDTH)
    ui.settingsTab = 'oauth' // 2 番目（100〜200）は窓の左にはみ出している
    await flushPromises()
    expect(list()?.scrollLeft).toBe(100)
    expect(TAB_COUNT * TAB_WIDTH).toBeGreaterThan(LIST_WIDTH)
    w.unmount()
  })

  test('横にスクロールしない幅（デスクトップの 1280）では、タブ行を動かさない', async () => {
    const { w, ui, list } = await mountSettings(1280, false)
    ui.openSettings('ai')
    await flushPromises()
    ui.settingsTab = 'general'
    await flushPromises()
    expect(list()?.scrollLeft).toBe(0)
    w.unmount()
  })
})
