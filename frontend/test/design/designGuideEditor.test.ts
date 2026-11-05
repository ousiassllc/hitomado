// Ousiass デザインガイド準拠の回帰テスト（#132〜）。作成画面（本文エディタ・リンク入力・宛先・添付）の部品・アイコン・リップルの規則を確かめる。
import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AttachmentBar from '../../src/components/compose/AttachmentBar.vue'
import ComposeFields from '../../src/components/compose/ComposeFields.vue'
import EditorLinkForm from '../../src/components/compose/EditorLinkForm.vue'
import RecipientField from '../../src/components/compose/RecipientField.vue'
import RichTextEditor from '../../src/components/compose/RichTextEditor.vue'
import { useComposeStore } from '../../src/stores/compose'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

describe('本文エディタのツールバー（I・M・#132・#133）', () => {
  test('太字・リンク・箇条書きは、すべて Iconoir のアイコン（svg）で文字を使わず、リップルがある', async () => {
    const w = mount(RichTextEditor, { props: { html: '' }, attachTo: document.body })
    await flushPromises()
    const buttons = w.findAll('[role="toolbar"] button')
    expect(buttons).toHaveLength(3)
    for (const b of buttons) {
      expect(b.find('svg').exists()).toBe(true)
      expect(b.text()).toBe('')
      expect(b.attributes('aria-label')).toBeTruthy()
      expect(b.classes()).toContain('has-ripple')
    }
    w.unmount()
  })
})

describe('リンク入力のボタン（M4・O7・#133・#134）', () => {
  test('適用・解除・キャンセルは BaseButton（小・SemiBold・リップルつき）', () => {
    const w = mount(EditorLinkForm, { props: { initial: '' } })
    const buttons = w.findAll('button')
    expect(buttons.map((b) => b.attributes('type'))).toEqual(['submit', 'button', 'button'])
    for (const b of buttons) {
      expect(b.classes()).toEqual(expect.arrayContaining(['btn', 'btn-sm', 'has-ripple']))
    }
  })
})

describe('Cc／Bcc の切り替え（M4・O7・#133・#134）', () => {
  test('BaseButton（小・ghost・リップルつき）で、押すと欄が開く', async () => {
    const store = useComposeStore()
    store.session = {
      sessionId: 's1',
      mode: 'new',
      senderLocked: false,
      content: { to: [], cc: null, bcc: null, subject: '', bodyHtml: '' },
      attachments: [],
    }
    const w = mount(ComposeFields, { props: { frozen: false }, attachTo: document.body })
    const toggles = w.findAll('.toggles button')
    expect(toggles).toHaveLength(2)
    for (const b of toggles) {
      expect(b.classes()).toEqual(
        expect.arrayContaining(['btn', 'btn-sm', 'btn-ghost', 'has-ripple']),
      )
    }
    await toggles[0].trigger('click')
    expect(w.findAll('.recipient-field')).toHaveLength(2)
    w.unmount()
  })
})

describe('チップ・添付の削除ボタン（M4・#133）', () => {
  test('宛先チップの × にリップルがある', () => {
    const w = mount(RecipientField, {
      props: { label: '宛先', modelValue: [{ email: 'ok@example.com' }] },
    })
    expect(w.get('.chip-remove').classes()).toContain('has-ripple')
  })

  test('添付の × にリップルがある', () => {
    const store = useComposeStore()
    store.attachments = [{ id: 'f1', filename: 'a.pdf', size: 10, source: 'local' }]
    const w = mount(AttachmentBar)
    expect(w.get('.attach-remove').classes()).toContain('has-ripple')
  })
})
