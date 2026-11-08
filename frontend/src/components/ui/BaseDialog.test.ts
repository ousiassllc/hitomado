import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BaseDialog from './BaseDialog.vue'
import ConfirmDialog from './ConfirmDialog.vue'

// 本文にフォーカスできる要素が無いダイアログは、本文を Tab で入れる名前付きの region にする（ガイド B6）。
beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const body = () => document.body.querySelector('.dlg-body')

describe('BaseDialog の bodyLabel', () => {
  test('指定すると本文が tabindex=0・role=region・aria-label を持つ', async () => {
    const w = mount(BaseDialog, {
      props: { open: true, title: '確認', bodyLabel: '確認' },
      slots: { default: '<p>本文</p>' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(body()?.getAttribute('tabindex')).toBe('0')
    expect(body()?.getAttribute('role')).toBe('region')
    expect(body()?.getAttribute('aria-label')).toBe('確認')
    w.unmount()
  })

  test('指定しなければ本文はフォーカス対象にならない（フォーカス可能な子を持つダイアログ）', async () => {
    const w = mount(BaseDialog, {
      props: { open: true, title: '設定' },
      slots: { default: '<input />' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(body()?.hasAttribute('tabindex')).toBe(false)
    expect(body()?.hasAttribute('role')).toBe(false)
    expect(body()?.hasAttribute('aria-label')).toBe(false)
    w.unmount()
  })
})

describe('ConfirmDialog', () => {
  test('補足（detail）がテキストだけのとき、本文はタイトルを領域名にした region', async () => {
    const w = mount(ConfirmDialog, {
      props: { open: true, title: '削除しますか', detail: '元に戻せません', confirmLabel: '削除' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(body()?.getAttribute('tabindex')).toBe('0')
    expect(body()?.getAttribute('aria-label')).toBe('削除しますか')
    w.unmount()
  })

  test('補足が無ければ本文は空なので region にしない', async () => {
    const w = mount(ConfirmDialog, {
      props: { open: true, title: '削除しますか', confirmLabel: '削除' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(body()?.hasAttribute('tabindex')).toBe(false)
    w.unmount()
  })
})
