import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseDialog from '../ui/BaseDialog.vue'
import { setup } from './ai/aiTestSupport'
import SettingsDialog from './SettingsDialog.vue'

// 本文のスクロールバーを 1 秒出す合図（BaseDialog の scrollKey → v-overlay-scroll の flashOn）。
// AI のタブの中で内側のタブ・ページを切り替えたときも、外側のタブを切り替えたときと同じく合図が変わる。
function mountOpen(tab: string) {
  const { pinia } = setup() // AI の設定（OpenAI につないである）と、モデル取得の空振りを入れる
  useSettingsStore().load = async () => {} // 開いた時の load() は Wails ランタイムを呼ぶので空振りさせる
  const ui = useUiStore()
  const w = mount(SettingsDialog, { global: { plugins: [pinia] }, attachTo: document.body })
  ui.openSettings(tab)
  const key = async (): Promise<string | undefined> => {
    await w.vm.$nextTick()
    return w.getComponent(BaseDialog).props('scrollKey')
  }
  return { w, ui, key }
}

describe('SettingsDialog の scrollKey', () => {
  test('AI 以外のタブは、タブの値そのまま', async () => {
    const { w, ui, key } = mountOpen('accounts')
    expect(await key()).toBe('accounts')
    ui.settingsTab = 'oauth'
    expect(await key()).toBe('oauth')
    w.unmount()
  })

  test('AI のタブでは、内側のタブ（つなぎ先／機能ごと）を切り替えると変わる', async () => {
    const { w, ui, key } = mountOpen('ai')
    const first = await key()
    expect(first).toBe('ai::')
    ui.aiTab = 'connect'
    const connect = await key()
    ui.aiTab = 'use'
    const use = await key()
    expect(new Set([first, connect, use]).size).toBe(3)
    w.unmount()
  })

  test('AI のタブでは、プロバイダのページを開く・戻ると変わる', async () => {
    const { w, ui, key } = mountOpen('ai')
    ui.aiTab = 'connect'
    const list = await key()
    ui.aiProviderPage = 'chatgpt'
    const page = await key()
    expect(page).not.toBe(list)
    ui.aiProviderPage = undefined
    expect(await key()).toBe(list)
    w.unmount()
  })
})
