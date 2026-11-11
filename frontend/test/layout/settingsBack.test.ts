// 設定の AI タブでプロバイダのページを開いているときの戻る（Android の戻るボタン）: ダイアログを閉じずに一覧へ戻り、
// 一覧（ページなし）の次の戻るでダイアログを閉じる。別のダイアログが前面にあるときは、そちらを閉じる。
import { afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import { effectScope } from 'vue'
import { useNavigation } from '../../src/composables/useNavigation'
import { useUiStore } from '../../src/stores/ui'

function setup() {
  setActivePinia(createPinia())
  const ui = useUiStore()
  const scope = effectScope()
  const nav = scope.run(() => useNavigation()) as ReturnType<typeof useNavigation>
  return { ui, nav }
}

const escapes: KeyboardEvent[] = []
const onKey = (e: Event) => escapes.push(e as KeyboardEvent)

beforeEach(() => {
  escapes.length = 0
  document.addEventListener('keydown', onKey)
  document.body.innerHTML = ''
})
afterEach(() => document.removeEventListener('keydown', onKey))

describe('設定の AI のプロバイダのページと戻る（#315）', () => {
  test('プロバイダのページ → 戻る でダイアログを閉じずに一覧へ。一覧で戻ると Esc（ダイアログを閉じる）', () => {
    const { ui, nav } = setup()
    ui.openSettings('ai')
    ui.enterOverlay(Symbol('settings'))
    ui.aiProviderPage = 'anthropic'

    expect(nav.goBack()).toBe(true)
    expect(ui.aiProviderPage).toBeUndefined()
    expect(ui.dialogs.settings).toBe(true)
    expect(escapes).toHaveLength(0)

    expect(nav.goBack()).toBe(true) // 一覧では今までどおりダイアログを閉じる（Esc と同じ経路）
    expect(escapes.map((e) => e.key)).toEqual(['Escape'])
  })

  test('AI 以外のタブ・ほかのダイアログが前面のときは、ページを戻さずダイアログを閉じる', () => {
    const { ui, nav } = setup()
    ui.openSettings('general')
    ui.enterOverlay(Symbol('settings'))
    ui.aiProviderPage = 'anthropic'
    expect(nav.goBack()).toBe(true)
    expect(escapes.map((e) => e.key)).toEqual(['Escape'])
    expect(ui.aiProviderPage).toBe('anthropic')

    escapes.length = 0
    ui.openSettings('ai')
    ui.aiProviderPage = 'openai'
    ui.enterOverlay(Symbol('consent')) // 同意の画面などが前面
    expect(nav.goBack()).toBe(true)
    expect(escapes.map((e) => e.key)).toEqual(['Escape'])
    expect(ui.aiProviderPage).toBe('openai')
  })

  test('設定を開き直すと、AI はプロバイダの一覧から始まる', () => {
    const { ui } = setup()
    ui.aiProviderPage = 'openai'
    ui.openSettings('ai')
    expect(ui.aiProviderPage).toBeUndefined()
  })
})
