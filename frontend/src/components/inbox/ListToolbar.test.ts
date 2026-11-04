import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ListToolbar from './ListToolbar.vue'

function mountToolbar(searchQuery?: string) {
  setActivePinia(createPinia())
  return mount(ListToolbar, {
    props: {
      selectedCount: 0,
      refreshing: false,
      searchQuery,
      labelEnabled: true,
      labels: [],
      labelsLoading: false,
    },
  })
}

describe('ListToolbar の検索結果モード', () => {
  test('検索語は繰り返さず、「受信箱に戻る」だけを出す（語は検索ボックスに残る）', async () => {
    const w = mountToolbar('from:a@example.com')
    expect(w.text()).not.toContain('検索結果')
    expect(w.text()).not.toContain('from:a@example.com')
    await w.get('button.back').trigger('click')
    expect(w.emitted('backToInbox')).toHaveLength(1)
    w.unmount()
  })

  test('検索中でなければ「受信箱に戻る」を出さない', () => {
    const w = mountToolbar(undefined)
    expect(w.find('button.back').exists()).toBe(false)
    w.unmount()
  })
})
