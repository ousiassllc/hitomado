// Ousiass デザインガイド準拠の回帰テスト（#128）。検索欄の構造を確かめる。
import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import SearchBar from '../../src/components/inbox/SearchBar.vue'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
import { rule } from './styleRule'

describe('部品・オーバーレイ（P・#128）', () => {
  test('検索欄は左に検索アイコンを持つ', () => {
    const w = mount(SearchBar, { props: { active: false, query: '' } })
    expect(w.get('.field > svg').attributes('aria-hidden')).toBe('true')
    expect(w.find('.field > input').exists()).toBe(true)
  })

  test('検索欄はブラウザ標準の✕を消し、右の自前のクリアボタンに一本化する（O8・#229）', () => {
    expect(rule('inbox/SearchBar.vue', '.input::-webkit-search-cancel-button {')).toContain(
      'appearance: none',
    )
  })
})

describe('フォームのグループ見出し（P・#319）', () => {
  test('「データの送信先」は fieldset の既定の枠を消し、中身を --surface 地・1px --border・--r-md のパネルに入れる', () => {
    const w = mount(AIDestinationBox, {
      props: { provider: 'openai', draftProvider: 'openai', draftBaseUrl: '' },
    })
    expect(w.get('fieldset > legend').text()).not.toBe('')
    expect(w.find('fieldset > .dest-panel [data-testid="ai-destination"]').exists()).toBe(true)
    expect(rule('settings/ai/AIDestinationBox.vue', '.dest {')).toContain('border: 0')
    const panel = rule('settings/ai/AIDestinationBox.vue', '.dest-panel {')
    expect(panel).toContain('border: 1px solid var(--border)')
    expect(panel).toContain('border-radius: var(--r-md)')
    expect(panel).toContain('background: var(--surface)')
  })

  test('legend は h4 の段（sm・SemiBold）', () => {
    const title = rule('settings/ai/AIDestinationBox.vue', '.dest-title {')
    expect(title).toContain('font-size: var(--font-size-sm)')
    expect(title).toContain('font-weight: 600')
  })
})

describe('ダイアログのフッター（#318）', () => {
  test('狭い幅ではボタンを折り返し、縮めて切れさせない', () => {
    expect(rule('ui/BaseDialog.vue', '.dlg-foot {')).toContain('flex-wrap: wrap')
  })
})
