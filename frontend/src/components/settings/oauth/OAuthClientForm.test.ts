import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import OAuthClientForm from './OAuthClientForm.vue'

describe('OAuthClientForm の見出し', () => {
  test('ダイアログタイトル（h2）の直下なので h3 にする（h4 で h3 を飛ばさない）', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const w = mount(OAuthClientForm, { global: { plugins: [pinia] } })
    expect(w.find('h3.title').exists()).toBe(true)
    expect(w.find('h4').exists()).toBe(false)
  })
})
