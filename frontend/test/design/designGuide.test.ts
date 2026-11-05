// Ousiass デザインガイド準拠の回帰テスト（#119〜#124）。マウントして、振る舞い・DOM を確かめる。
import { afterEach, describe, expect, jest, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h, nextTick } from 'vue'
import AlertStack from '../../src/components/shell/AlertStack.vue'
import ToastHost from '../../src/components/shell/ToastHost.vue'
import BaseButton from '../../src/components/ui/BaseButton.vue'
import BaseDialog from '../../src/components/ui/BaseDialog.vue'
import BaseMenu from '../../src/components/ui/BaseMenu.vue'
import ConfirmDialog from '../../src/components/ui/ConfirmDialog.vue'
import Spinner from '../../src/components/ui/Spinner.vue'
import { useUiStore } from '../../src/stores/ui'

afterEach(() => {
  jest.useRealTimers()
  document.body.innerHTML = ''
})

describe('トースト（O3）: 同時に 1 つ・3 秒・短い結果だけ', () => {
  test('新しいトーストは前のものを置き換える（積まない）', () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    ui.pushToast('success', 'A')
    ui.pushToast('info', 'B')
    expect(ui.toast).toMatchObject({ kind: 'info', message: 'B' })
  })

  test('3 秒で消える', async () => {
    jest.useFakeTimers()
    setActivePinia(createPinia())
    const ui = useUiStore()
    const w = mount(ToastHost, { attachTo: document.body })
    ui.pushToast('success', '保存しました')
    await nextTick()
    expect(w.find('.toast').exists()).toBe(true)
    jest.advanceTimersByTime(2999)
    expect(ui.toast).toBeDefined()
    jest.advanceTimersByTime(1)
    expect(ui.toast).toBeUndefined()
    w.unmount()
  })

  test('失敗はトーストではなくアラートに出る（再試行つきでもトーストに載らない）', () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    ui.pushAlert('読み込めませんでした', () => {})
    expect(ui.toast).toBeUndefined()
    expect(ui.alerts).toHaveLength(1)
  })
})

describe('インラインアラート', () => {
  test('同じ文言は重ねず、新しいものへ置き換える', () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    ui.pushAlert('失敗')
    ui.pushAlert('失敗')
    ui.pushAlert('別の失敗')
    expect(ui.alerts.map((a) => a.message)).toEqual(['失敗', '別の失敗'])
  })

  test('アイコン・見出し・本文を出し、再試行は閉じてから実行する', async () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    let retried = 0
    ui.pushAlert('通信に失敗しました', () => retried++)
    const w = mount(AlertStack, { attachTo: document.body })
    const alert = w.get('[role="alert"]')
    expect(alert.find('svg').exists()).toBe(true)
    expect(alert.text()).toContain('うまくいきませんでした')
    expect(alert.text()).toContain('通信に失敗しました')
    await w
      .findAll('button')
      .find((b) => b.text() === '再試行')
      ?.trigger('click')
    expect(retried).toBe(1)
    expect(ui.alerts).toHaveLength(0)
    w.unmount()
  })

  test('ダイアログが開いている間は、最前面のダイアログの中に出る（スクラムの裏に隠れない）', async () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    ui.pushAlert('保存できません')
    const w = mount(
      {
        render: () => h(BaseDialog, { open: true, title: 'T' }, { default: () => h('p', 'body') }),
      },
      { attachTo: document.body },
    )
    await flushPromises()
    expect(document.querySelector('[role="dialog"] [role="alert"]')?.textContent).toContain(
      '保存できません',
    )
    w.unmount()
    expect(ui.overlays).toHaveLength(0)
  })
})

describe('モーダル（O1・O2）', () => {
  test('確認ダイアログは外側クリックと同じ update:open(false) で閉じ、確認ボタンで confirm を出す', async () => {
    const w = mount(ConfirmDialog, {
      props: { open: true, title: '削除しますか？', confirmLabel: '削除する' },
      attachTo: document.body,
    })
    await flushPromises()
    const buttons = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')]
    buttons.find((b) => b.textContent?.trim() === '削除する')?.click()
    expect(w.emitted('confirm')).toHaveLength(1)
    buttons.find((b) => b.textContent?.trim() === 'キャンセル')?.click()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  test('closable=false は閉じるボタンが無く、Esc でも閉じる要求を出さない（明示のボタンだけで閉じる確認）', async () => {
    setActivePinia(createPinia())
    const w = mount(BaseDialog, {
      props: { open: true, title: '確認', closable: false },
      attachTo: document.body,
    })
    await nextTick()
    expect(document.querySelector('button[aria-label="閉じる"]')).toBeNull()
    document
      .querySelector('[role="dialog"]')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(w.emitted('update:open')).toBeUndefined()
    w.unmount()
  })

  test('実行中（busy）の確認ダイアログは、キャンセルも閉じる操作も無効', async () => {
    const w = mount(ConfirmDialog, {
      props: { open: true, title: 't', confirmLabel: '削除する', busy: true },
      attachTo: document.body,
    })
    await flushPromises()
    const cancel = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
      (b) => b.textContent?.trim() === 'キャンセル',
    )
    expect(cancel?.disabled).toBe(true)
    w.unmount()
  })
})

describe('アイコン（I）', () => {
  test('ダイアログの閉じるボタンは Iconoir の SVG（文字の ×）', async () => {
    const w = mount(BaseDialog, { props: { open: true, title: 'T' }, attachTo: document.body })
    await nextTick()
    const close = document.querySelector('button[aria-label="閉じる"]')
    expect(close?.querySelector('svg')).not.toBeNull()
    expect(close?.textContent?.trim()).toBe('')
    w.unmount()
  })

  test('チェック付きのメニュー項目は menuitemcheckbox で、選択中だけ Check アイコンが出る', async () => {
    setActivePinia(createPinia())
    const w = mount(
      {
        render: () =>
          h(
            BaseMenu,
            {
              items: [
                { key: 'a', label: 'A', checked: true },
                { key: 'b', label: 'B', checked: false },
              ],
            },
            { trigger: () => h('button', { id: 't' }, 'menu') },
          ),
      },
      { attachTo: document.body },
    )
    await flushPromises()
    const trigger = document.querySelector<HTMLElement>('#t')
    trigger?.focus()
    trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()
    const items = [...document.querySelectorAll('[role="menuitemcheckbox"]')]
    expect(items.map((i) => i.getAttribute('aria-checked'))).toEqual(['true', 'false'])
    expect(items.map((i) => i.querySelector('svg') !== null)).toEqual([true, false])
    w.unmount()
  })
})

describe('モーション（M）', () => {
  test('スピナーの大きさは --icon-sm/md/lg（既定 20px）', () => {
    const w = mount(Spinner)
    expect(w.classes()).toContain('spinner-md')
    expect(mount(Spinner, { props: { size: 'lg' } }).classes()).toContain('spinner-lg')
  })

  test('ボタンの押下でリップルが出て、無効のときは出ない', async () => {
    const w = mount(BaseButton, { slots: { default: '送信' }, attachTo: document.body })
    await w.trigger('pointerdown', { clientX: 4, clientY: 4 })
    expect(w.element.querySelectorAll('.ripple')).toHaveLength(1)
    w.unmount()
    const off = mount(BaseButton, { props: { disabled: true }, slots: { default: '送信' } })
    await off.trigger('pointerdown')
    expect(off.element.querySelectorAll('.ripple')).toHaveLength(0)
  })
})
