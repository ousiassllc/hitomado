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

// 開いた直後の初期フォーカスは、× ではなくダイアログ本体に置く（#376・#377）。× に置くと、開いただけで :focus-visible のリングが出る。
// Tab を 1 回押したときの × のリングは、ブラウザでの表示で確かめる（ここはフォーカスの移動だけを確かめる）。
describe('BaseDialog の初期フォーカス・Esc・フォーカスの戻し', () => {
  const dialog = () => document.body.querySelector<HTMLElement>('.dlg')
  const close = () => document.body.querySelector<HTMLElement>('.dlg-close')

  test('開くと、フォーカスはダイアログ本体（tabindex=-1）にあり、× ではない', async () => {
    const w = mount(BaseDialog, {
      props: { open: true, title: '設定' },
      slots: { default: '<input />' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(dialog()?.getAttribute('tabindex')).toBe('-1')
    expect(document.activeElement).toBe(dialog())
    expect(document.activeElement).not.toBe(close())
    expect(close()).not.toBeNull()
    w.unmount()
  })

  test('busy で × が無くても、フォーカスは本体に置く', async () => {
    const w = mount(BaseDialog, {
      props: { open: true, title: '送信中', busy: true },
      slots: { default: '<input />' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(close()).toBeNull()
    expect(document.activeElement).toBe(dialog())
    w.unmount()
  })

  test('Esc で update:open(false) を出す', async () => {
    const w = mount(BaseDialog, {
      props: { open: true, title: '設定' },
      slots: { default: '<input />' },
      attachTo: document.body,
    })
    await flushPromises()
    dialog()?.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    await flushPromises()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })

  test('閉じると、開く前にフォーカスのあった要素へフォーカスが戻る', async () => {
    const trigger = document.createElement('button')
    document.body.append(trigger)
    trigger.focus()
    const w = mount(BaseDialog, {
      props: { open: false, title: '設定' },
      slots: { default: '<input />' },
      attachTo: document.body,
    })
    await w.setProps({ open: true })
    await flushPromises()
    expect(document.activeElement).toBe(dialog())
    await w.setProps({ open: false })
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(document.activeElement).toBe(trigger)
    w.unmount()
  })
})
