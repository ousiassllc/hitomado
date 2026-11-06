import { afterAll, afterEach, beforeAll, describe, expect, test } from 'bun:test'
import { compileStyle, parse } from '@vue/compiler-sfc'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h } from 'vue'
import BaseDialog from '../src/components/ui/BaseDialog.vue'
import BaseMenu from '../src/components/ui/BaseMenu.vue'
import BaseSelect from '../src/components/ui/BaseSelect.vue'
import BaseTooltip from '../src/components/ui/BaseTooltip.vue'

setActivePinia(createPinia())

/**
 * テストの SFC ローダーは <style> を取り込まない。本番（vite の vue プラグイン）と同じく scoped を適用した CSS と
 * data-v-* 属性を再現して、「ポップアップ本体にスタイルが届くか」を getComputedStyle で確かめる。
 */
async function withScopedStyles(
  component: object,
  file: string,
  scopeId: string,
): Promise<() => void> {
  const path = new URL(`../src/components/ui/${file}`, import.meta.url).pathname
  const { descriptor } = parse(await Bun.file(path).text(), { filename: path })
  const styles = descriptor.styles.map((s) => {
    const out = compileStyle({
      source: s.content,
      filename: path,
      id: scopeId,
      scoped: s.scoped === true,
    })
    const el = document.createElement('style')
    el.textContent = out.code
    document.head.append(el)
    return el
  })
  const target = component as { __scopeId?: string }
  const before = target.__scopeId
  target.__scopeId = scopeId
  // 同じプロセスで走る他のテストに data-v-* 属性やスタイルを残さない。
  return () => {
    for (const el of styles) el.remove()
    if (before === undefined) delete target.__scopeId
    else target.__scopeId = before
  }
}

const restorers: Array<() => void> = []
beforeAll(async () => {
  // z-index は --z-* トークン（styles/tokens.css）で決まるので、トークンも当てる。
  const tokens = document.createElement('style')
  tokens.textContent = await Bun.file(
    new URL('../src/styles/tokens.css', import.meta.url).pathname,
  ).text()
  document.head.append(tokens)
  restorers.push(
    () => tokens.remove(),
    await withScopedStyles(BaseSelect, 'BaseSelect.vue', 'data-v-select'),
    await withScopedStyles(BaseMenu, 'BaseMenu.vue', 'data-v-menu'),
    await withScopedStyles(BaseTooltip, 'BaseTooltip.vue', 'data-v-tooltip'),
    await withScopedStyles(BaseDialog, 'BaseDialog.vue', 'data-v-dialog'),
  )
})
afterAll(() => {
  for (const restore of restorers) restore()
})

const options = [
  { value: 'a', label: 'Alpha' },
  { value: 'b', label: 'Beta' },
]

let wrapper: VueWrapper | undefined
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  document.body.removeAttribute('style')
})

/** ダイアログ（z-index 400）の中に popup を出す部品を置く。 */
function mountInDialog(inner: () => ReturnType<typeof h>): VueWrapper {
  return mount(
    { render: () => h(BaseDialog, { open: true, title: 'T' }, { default: inner }) },
    { attachTo: document.body },
  )
}

function zIndexOf(el: Element): number {
  return Number(getComputedStyle(el).zIndex)
}

/** ダイアログ本体の z-index。0 のまま比較が素通りしないよう 400（--z-modal）であることも確かめる。 */
function dialogZIndex(): number {
  const z = zIndexOf(document.querySelector('.dlg') as Element)
  expect(z).toBe(400)
  return z
}

describe('ダイアログ内のポップアップ（#98）', () => {
  test('BaseSelect: 開いた一覧がダイアログ（z-index 400）より前面', async () => {
    wrapper = mountInDialog(() =>
      h(BaseSelect, { modelValue: 'a', options, ariaLabel: 'provider' }),
    )
    await flushPromises()
    const trigger = document.querySelector<HTMLElement>('[aria-label="provider"]')
    expect(trigger).not.toBeNull()
    trigger?.focus()
    trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    const list = document.querySelector('[role="listbox"]')
    expect(list).not.toBeNull()
    expect(document.querySelectorAll('[role="option"]')).toHaveLength(2)
    expect(zIndexOf(list as Element)).toBeGreaterThan(dialogZIndex())
  })

  test('BaseMenu: 開いたメニューがダイアログより前面', async () => {
    wrapper = mountInDialog(() =>
      h(
        BaseMenu,
        { items: [{ key: 'x', label: 'X' }] },
        { trigger: () => h('button', { id: 't' }, 'menu') },
      ),
    )
    await flushPromises()
    const trigger = document.querySelector<HTMLElement>('#t')
    trigger?.focus()
    trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    const menu = document.querySelector('[role="menu"]')
    expect(menu).not.toBeNull()
    expect(zIndexOf(menu as Element)).toBeGreaterThan(dialogZIndex())
  })
})

// reka の Fragment / Portal 配下の要素（SelectViewport・Popper 本体）には scoped 属性（data-v-*）が付かない。
// 実際に描画して、scope 属性が無い要素にも余白（リングが収まる --s-1 = 4px）が当たることを確かめる（#252）。
describe('スクロール領域の余白（外側リングが収まる 4px）', () => {
  test('BaseSelect: SelectViewport は scope 属性が無くても padding が当たる', async () => {
    wrapper = mount(BaseSelect, {
      props: { modelValue: 'a', options, ariaLabel: 'provider' },
      attachTo: document.body,
    })
    await flushPromises()
    const trigger = document.querySelector<HTMLElement>('[aria-label="provider"]')
    trigger?.focus()
    trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    const viewport = document.querySelector('.sel-viewport') as Element
    expect(viewport).not.toBeNull()
    expect(viewport.hasAttribute('data-v-select')).toBe(false) // scoped セレクタでは当たらない要素であることの確認
    expect(getComputedStyle(viewport).padding).toBe('4px')
  })

  test('BaseMenu: .menu-pop は scope 属性が無くても padding が当たる', async () => {
    wrapper = mount(BaseMenu, {
      props: { items: [{ key: 'x', label: 'X' }] },
      slots: { trigger: () => h('button', { id: 't' }, 'menu') },
      attachTo: document.body,
    })
    await flushPromises()
    const trigger = document.querySelector<HTMLElement>('#t')
    trigger?.focus()
    trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    const pop = document.querySelector('.menu-pop') as Element
    expect(pop).not.toBeNull()
    expect(pop.hasAttribute('data-v-menu')).toBe(false)
    expect(getComputedStyle(pop).padding).toBe('4px')
  })
})

test('BaseTooltip: 出したツールチップがダイアログより前面', async () => {
  wrapper = mountInDialog(() =>
    h(BaseTooltip, { text: 'ヒント' }, () => h('button', { id: 't' }, 'b')),
  )
  await flushPromises()
  document.querySelector<HTMLElement>('#t')?.focus()
  await flushPromises()

  const tip = document.querySelector('[data-reka-popper-content-wrapper] > [data-state]')
  expect(tip).not.toBeNull()
  expect(zIndexOf(tip as Element)).toBeGreaterThan(dialogZIndex())
})
