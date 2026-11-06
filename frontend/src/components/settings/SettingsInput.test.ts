import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import SettingsInput from './SettingsInput.vue'

describe('SettingsInput の mono', () => {
  test('既定では mono クラスを付けない（文章入力は sans のまま）', () => {
    const w = mount(SettingsInput, { props: { modelValue: '', label: '表示名' } })
    expect(w.get('input').classes()).not.toContain('mono')
  })

  test('mono を指定した入力だけ mono クラスを付ける', () => {
    const w = mount(SettingsInput, {
      props: { modelValue: '', label: 'クライアント ID', mono: true },
    })
    expect(w.get('input').classes()).toContain('mono')
  })
})
