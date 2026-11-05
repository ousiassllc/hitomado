// Ousiass デザインガイド準拠の回帰テスト（#119〜#124）。見た目そのものではなく、守りたい規則を確かめる。
import { afterEach, describe, expect, jest, test } from 'bun:test'
import { readFileSync } from 'node:fs'
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

const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')

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

  test('スマホ幅でも全画面にしない（中央モーダルのまま幅だけ縮める）', () => {
    const css = read('../../src/components/ui/BaseDialog.vue')
    expect(css).not.toContain('dlg-full')
    expect(css).toContain('calc(100vw - 32px)')
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

  test('文字記号のアイコン（× ▾ ▸ ✓ ☰ ←）をテンプレートに残さない', () => {
    const files = [
      'ui/BaseDialog.vue',
      'ui/BaseSelect.vue',
      'inbox/SearchBar.vue',
      'inbox/ThreadListPane.vue',
      'thread/ReplyBar.vue',
      'thread/MessageCard.vue',
      'shell/BackToListButton.vue',
      'shell/ToastHost.vue',
      'settings/accounts/ColorPicker.vue',
    ]
    for (const f of files) {
      const template =
        /<template>[\s\S]*<\/template>/.exec(read(`../../src/components/${f}`))?.[0] ?? ''
      expect(template).not.toMatch(/[×▾▸✓☰←]/)
    }
  })
})

describe('モーション（M）', () => {
  test('スピナー（処理中の表示）は prefers-reduced-motion でも止めない', () => {
    const spinner = read('../../src/components/ui/Spinner.vue')
    expect(spinner).toContain('animation: spin')
    expect(spinner).not.toMatch(/prefers-reduced-motion[\s\S]*animation:\s*none/)
    const tokens = read('../../src/styles/tokens.css')
    expect(tokens).not.toMatch(/animation-duration:\s*0ms\s*!important/)
    expect(tokens).not.toMatch(/animation:\s*none/)
  })

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

describe('トークン（C・T・S・O）', () => {
  const css = read('../../src/styles/tokens.css')
  const root = css.slice(0, css.indexOf('@media (prefers-color-scheme: dark)'))
  const token = (name: string) => new RegExp(`${name}:\\s*([^;]+);`).exec(root)?.[1]?.trim()

  test('色はガイドのコアトークン。アクセントはインク色で、青の独自色を持たない', () => {
    expect(token('--accent')).toBe('#050505')
    expect(token('--focus')).toBe('#4a72d0')
    expect(token('--info')).toBe('#4a5a6a')
    expect(css).not.toMatch(/--color-/)
    expect(css).not.toMatch(/#1a56db/i)
  })

  test('サイズスケールは 12 / 14 / 16 / 18 / 24 / 32 / 40、本文は 16px', () => {
    expect(
      ['xs', 'sm', 'base', 'lg', 'xl', '2xl', '3xl'].map((k) => token(`--font-size-${k}`)),
    ).toEqual(['12px', '14px', '16px', '18px', '24px', '32px', '40px'])
    expect(read('../../src/styles/reset.css')).toMatch(/font-size: var\(--font-size-base\)/)
  })

  test('フォントは Sora + Noto Sans JP（等幅は JetBrains Mono）で、アプリに同梱する', () => {
    expect(token('--font-sans')).toContain('Sora')
    expect(token('--font-sans')).toContain('Noto Sans JP')
    expect(token('--font-mono')).toContain('JetBrains Mono')
    const app = read('../../src/App.vue')
    for (const pkg of ['sora', 'noto-sans-jp', 'jetbrains-mono'])
      expect(app).toContain(`@fontsource-variable/${pkg}`)
  })

  test('影はモーダルだけ（--shadow-modal）', () => {
    expect(token('--shadow-modal')).toBe('0 1px 2px rgb(0 0 0 / 6%)')
    expect(css).not.toContain('--shadow-popup')
  })

  test('z-index は --z-*（dropdown 100 / drawer 300 / modal 400 / toast 500）', () => {
    expect(['dropdown', 'drawer', 'modal', 'toast'].map((k) => token(`--z-${k}`))).toEqual([
      '100',
      '300',
      '400',
      '500',
    ])
  })

  test('コンポーネントは z-index・色・影・角丸を直書きしない', () => {
    const files = [
      'ui/BaseDialog.vue',
      'ui/BaseMenu.vue',
      'ui/BaseSelect.vue',
      'ui/BaseTooltip.vue',
      'shell/AppShell.vue',
      'shell/ToastHost.vue',
      'compose/RecipientField.vue',
    ]
    for (const f of files) {
      const style = /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${f}`))?.[0] ?? ''
      expect(style).not.toMatch(/z-index:\s*\d/)
      expect(style).not.toMatch(/#[0-9a-fA-F]{3,6}\b/)
      expect(style).not.toContain('shadow-popup')
    }
  })

  test('ボタンは通常 40px・小 32px、無効は opacity 0.4', () => {
    const btn = read('../../src/components/ui/BaseButton.vue')
    expect(btn).toContain('min-height: var(--control-h)')
    expect(btn).toContain('var(--control-h-sm)')
    expect(btn).toMatch(/opacity: 0\.4;/)
    expect(token('--control-h')).toBe('40px')
    expect(token('--control-h-sm')).toBe('32px')
  })
})

describe('フォームの focus（C3・#125）', () => {
  // 入力欄の focus は --focus の枠線 + --ring の 2px。外枠を持つもの（宛先・本文）は :focus-within で同じ指定にする。
  const focusRule = (rel: string, selector: string) => {
    const style = /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${rel}`))?.[0] ?? ''
    const escaped = selector.replace(/[.:]/g, '\\$&')
    return new RegExp(`${escaped}\\s*\\{([^}]*)\\}`).exec(style)?.[1] ?? ''
  }

  test.each([
    ['compose/ComposeFields.vue', '.subject-input:focus'],
    ['compose/ai/AiDraftInstruction.vue', '.instruction-input:focus'],
    ['compose/RecipientField.vue', '.recipient-box:focus-within'],
    ['compose/RichTextEditor.vue', '.rte:focus-within'],
    ['settings/SettingsInput.vue', '.field-input:focus'],
    ['inbox/SearchBar.vue', '.input:focus'],
  ])('%s の %s は --focus の枠線と --ring の 2px を使う', (rel, selector) => {
    const body = focusRule(rel, selector)
    expect(body).toContain('border-color: var(--focus)')
    expect(body).toContain('box-shadow: 0 0 0 2px var(--ring)')
  })
})

describe('タイポグラフィ（T・#126）', () => {
  const reset = read('../../src/styles/reset.css')
  const rule = (css: string, selector: string) =>
    new RegExp(`${selector}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? ''

  test('見出し h1〜h6 は SemiBold（600）、見出しでない <strong> は 400', () => {
    expect(rule(reset, 'h1,\\s*h2,\\s*h3,\\s*h4,\\s*h5,\\s*h6')).toContain('font-weight: 600')
    expect(rule(reset, 'strong')).toContain('font-weight: 400')
  })

  test('本文エディタで利用者が付けた太字は 400 に負けず太字で見える', () => {
    const style = read('../../src/components/compose/RichTextEditor.vue')
    expect(rule(style, '\\.rte-body :deep\\(\\.editor-content strong\\)')).toContain(
      'font-weight: 700',
    )
  })

  test.each([
    ['settings/McpSettings.vue', '\\.token'],
    ['settings/McpSettings.vue', '\\.path'],
    ['settings/oauth/OAuthClientsSettings.vue', '\\.client-id'],
  ])('%s の %s（識別子・トークン）は JetBrains Mono（--font-mono）', (rel, selector) => {
    expect(rule(read(`../../src/components/${rel}`), selector)).toContain(
      'font-family: var(--font-mono)',
    )
  })
})

describe('入力欄の padding（S2・#127）', () => {
  const padding = (rel: string, selector: string) => {
    const style = /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${rel}`))?.[0] ?? ''
    return new RegExp(`${selector}\\s*\\{[^}]*?padding:\\s*([^;]+);`).exec(style)?.[1]
  }

  test.each([
    ['inbox/SearchBar.vue', '\\.input', '0 var(--s-3)'],
    ['settings/SettingsInput.vue', '\\.field-input', '0 var(--s-3)'],
    ['compose/ComposeFields.vue', '\\.subject-input', '0 var(--s-3)'],
    ['compose/ai/AiDraftInstruction.vue', '\\.instruction-input', 'var(--s-3)'],
    ['compose/RecipientField.vue', '\\.recipient-box', 'var(--s-1) var(--s-3)'],
  ])('%s の %s は左右 12px（--s-3）で揃える', (rel, selector, expected) => {
    expect(padding(rel, selector)).toBe(expected)
  })
})
