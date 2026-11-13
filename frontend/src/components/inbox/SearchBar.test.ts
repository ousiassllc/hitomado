import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SearchBar from './SearchBar.vue'

function mountBar(active: boolean, query = '') {
  setActivePinia(createPinia())
  return mount(SearchBar, { props: { active, query }, attachTo: document.body })
}

const CLEAR = 'button[aria-label="検索をクリア"]'

describe('SearchBar の右端のクリア（O8）', () => {
  test('クリアは入力欄（.field）の中にあり、text が空のあいだは出ない', async () => {
    const w = mountBar(false)
    expect(w.find(CLEAR).exists()).toBe(false)
    await w.get('input').setValue('from:a@example.com')
    expect(w.get('.field').find(CLEAR).exists()).toBe(true)
    w.unmount()
  })

  test('検索中でなくても入力があれば出る。押すと入力を空にして隠し、clear は出さない', async () => {
    const w = mountBar(false)
    await w.get('input').setValue('abc')
    await w.get(CLEAR).trigger('click')
    expect((w.get('input').element as HTMLInputElement).value).toBe('')
    expect(w.find(CLEAR).exists()).toBe(false)
    expect(w.emitted('clear')).toBeUndefined()
    expect(document.activeElement).toBe(w.get('input').element)
    w.unmount()
  })

  test('検索中に押すと入力を空にして clear を出す', async () => {
    const w = mountBar(true, 'from:a@example.com')
    expect(w.find(CLEAR).exists()).toBe(true)
    await w.get(CLEAR).trigger('click')
    expect((w.get('input').element as HTMLInputElement).value).toBe('')
    expect(w.emitted('clear')).toHaveLength(1)
    w.unmount()
  })
})

describe('SearchBar のプレースホルダ（#348）', () => {
  test('クリアが出ていないあいだは、クリアの分の右 padding を空けない（has-clear は入力があるときだけ）', async () => {
    const w = mountBar(false)
    expect(w.get('input').classes()).not.toContain('has-clear')
    await w.get('input').setValue('abc')
    expect(w.get('input').classes()).toContain('has-clear')
    await w.get(CLEAR).trigger('click')
    expect(w.get('input').classes()).not.toContain('has-clear')
    w.unmount()
  })
})
