// Ousiass デザインガイド準拠の回帰テスト（#132〜）。本文エディタのツールバー・リンク入力まわりの部品・アイコン・リップルの規則を確かめる。
import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RichTextEditor from '../../src/components/compose/RichTextEditor.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

describe('本文エディタのツールバー（I・#132）', () => {
  test('太字・リンク・箇条書きは、すべて Iconoir のアイコン（svg）で文字を使わない', async () => {
    const w = mount(RichTextEditor, { props: { html: '' }, attachTo: document.body })
    await flushPromises()
    const buttons = w.findAll('[role="toolbar"] button')
    expect(buttons).toHaveLength(3)
    for (const b of buttons) {
      expect(b.find('svg').exists()).toBe(true)
      expect(b.text()).toBe('')
      expect(b.attributes('aria-label')).toBeTruthy()
    }
    w.unmount()
  })
})
