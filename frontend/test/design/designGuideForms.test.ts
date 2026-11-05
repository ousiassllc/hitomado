// Ousiass デザインガイド準拠の回帰テスト（#128）。検索欄の構造を確かめる。
import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import SearchBar from '../../src/components/inbox/SearchBar.vue'

describe('部品・オーバーレイ（P・#128）', () => {
  test('検索欄は左に検索アイコンを持つ', () => {
    const w = mount(SearchBar, { props: { active: false, query: '' } })
    expect(w.get('.field > svg').attributes('aria-hidden')).toBe('true')
    expect(w.find('.field > input').exists()).toBe(true)
  })
})
