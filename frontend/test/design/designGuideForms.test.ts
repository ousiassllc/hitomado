// Ousiass デザインガイド準拠の回帰テスト（#128）。検索欄の構造を確かめる。
import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import SearchBar from '../../src/components/inbox/SearchBar.vue'
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
