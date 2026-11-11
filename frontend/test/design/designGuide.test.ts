// Ousiass デザインガイド準拠の回帰テスト（#119〜#124）。マウントして、振る舞い・DOM を確かめる。
import { afterEach, describe, expect, jest, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h, nextTick } from 'vue'
import AlertStack from '../../src/components/shell/AlertStack.vue'
import BaseButton from '../../src/components/ui/BaseButton.vue'
import BaseDialog from '../../src/components/ui/BaseDialog.vue'
import BaseDisclosure from '../../src/components/ui/BaseDisclosure.vue'
import BaseMenu from '../../src/components/ui/BaseMenu.vue'
import ConfirmDialog from '../../src/components/ui/ConfirmDialog.vue'
import Spinner from '../../src/components/ui/Spinner.vue'
import { useUiStore } from '../../src/stores/ui'

afterEach(() => {
  jest.useRealTimers()
  document.body.innerHTML = ''
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

  test('busy 中は閉じるボタンが無く、Esc・外側クリックでも閉じる要求を出さない。busy でなければ出す', async () => {
    setActivePinia(createPinia())
    const w = mount(BaseDialog, {
      props: { open: true, title: '確認', busy: true },
      attachTo: document.body,
    })
    await flushPromises()
    // Reka は pointerdown の購読を setTimeout(0) で始める。その購読が済んでから押す（購読の完了待ちで、時間の見込みではない）。
    await new Promise((r) => setTimeout(r, 0))
    const outside = () =>
      document
        .querySelector('.dlg-overlay')
        ?.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    const esc = () =>
      document
        .querySelector('[role="dialog"]')
        ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(document.querySelector('button[aria-label="閉じる"]')).toBeNull()
    esc()
    outside()
    await flushPromises()
    expect(w.emitted('update:open')).toBeUndefined()

    await w.setProps({ busy: false })
    expect(document.querySelector('button[aria-label="閉じる"]')).not.toBeNull()
    outside()
    await flushPromises()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    esc()
    await flushPromises()
    expect(w.emitted('update:open')).toHaveLength(2)
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

describe('アコーディオン（I1・I2）: シェブロンは Iconoir の 16px', () => {
  test('summary 内に Iconoir の SVG（icon-sm・aria-hidden）を置き、::after の擬似要素で描かない', () => {
    const w = mount(BaseDisclosure, { props: { summary: '接続方法' }, slots: { default: '本文' } })
    const chevron = w.get('summary svg')
    expect(chevron.classes()).toEqual(expect.arrayContaining(['icon', 'icon-sm']))
    expect(chevron.attributes('aria-hidden')).toBe('true')
    const style = parse(
      readFileSync(new URL('../../src/components/ui/BaseDisclosure.vue', import.meta.url), 'utf8'),
    ).descriptor.styles[0].content
    expect(style).not.toContain('::after')
  })
})

describe('モーション（M）', () => {
  test('スピナーの大きさは 20px（--icon-md）だけで、サイズの指定は無い（O12）', () => {
    const w = mount(Spinner)
    expect(w.classes()).toEqual(['spinner'])
    const style = parse(
      readFileSync(new URL('../../src/components/ui/Spinner.vue', import.meta.url), 'utf8'),
    ).descriptor.styles[0].content
    expect(style).toContain('width: var(--icon-md)')
    expect(style).toContain('height: var(--icon-md)')
    expect(style).not.toMatch(/--icon-(sm|lg)/)
  })

  test('スピナーは prefers-reduced-motion: reduce で回転を止め、リング（track と先端）は残す（M3）', () => {
    const style = parse(
      readFileSync(new URL('../../src/components/ui/Spinner.vue', import.meta.url), 'utf8'),
    ).descriptor.styles[0].content
    expect(style).toMatch(/\.spinner \{[^}]*animation: spin 0\.9s linear infinite;/)
    const reduced =
      style.match(
        /@media \(prefers-reduced-motion: reduce\) \{\s*\.spinner \{([^}]*)\}\s*\}/,
      )?.[1] ?? ''
    expect(reduced.replace(/\s+/g, '')).toBe('animation:none;')
    // 静止しても処理中だとわかるよう、リングの描画（border・先端の色）は reduced-motion で消さない
    expect(style).toMatch(
      /\.spinner \{[^}]*border: 2px solid var\(--accent-soft\);[^}]*border-top-color: var\(--accent\);/,
    )
  })

  test('primary ボタンの loading のスピナーは、--accent 地で見えるよう --bone に反転する（O12）', () => {
    const w = mount(BaseButton, {
      props: { variant: 'primary', loading: true },
      slots: { default: '送信' },
    })
    expect(w.find('.spinner').exists()).toBe(true)
    const style = parse(
      readFileSync(new URL('../../src/components/ui/BaseButton.vue', import.meta.url), 'utf8'),
    ).descriptor.styles[0].content
    const body =
      style.slice(style.indexOf('.btn-primary .spinner {')).match(/\{([^}]*)\}/)?.[1] ?? ''
    expect(body).toContain('border-top-color: var(--bone)')
    expect(body).toContain('color-mix(in srgb, var(--bone) 30%, transparent)')
    expect(body).not.toMatch(/width|height/)
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

  test('右ボタン・中ボタンの押下ではリップルが出ない（M4）', async () => {
    const w = mount(BaseButton, { slots: { default: '送信' }, attachTo: document.body })
    await w.trigger('pointerdown', { button: 2 })
    await w.trigger('pointerdown', { button: 1 })
    expect(w.element.querySelectorAll('.ripple')).toHaveLength(0)
    await w.trigger('pointerdown', { button: 0 })
    expect(w.element.querySelectorAll('.ripple')).toHaveLength(1)
    w.unmount()
  })
})

describe('危険ボタン（O7・#190）: 透明地・--danger の文字・--danger-border の枠', () => {
  const style = parse(
    readFileSync(new URL('../../src/components/ui/BaseButton.vue', import.meta.url), 'utf8'),
  ).descriptor.styles[0].content
  const rule = (selector: string) =>
    style.slice(style.indexOf(selector)).match(/\{([^}]*)\}/)?.[1] ?? ''

  test('variant="danger" は btn-danger クラスを付ける', () => {
    const w = mount(BaseButton, { props: { variant: 'danger' }, slots: { default: '削除' } })
    expect(w.classes()).toContain('btn-danger')
  })

  test('塗りつぶさない（透明地・文字 --danger・枠は --danger-border）', () => {
    const body = rule('.btn-danger {')
    expect(body).toContain('background: transparent')
    expect(body).toContain('color: var(--danger)')
    expect(body).toContain('border-color: var(--danger-border)')
    expect(body).not.toContain('var(--bone)')
  })

  test('ホバーは --danger-soft（--danger の 10%）の薄い地', () => {
    expect(rule('.btn-danger:hover:not(:disabled) {')).toContain('background: var(--danger-soft)')
  })
})
