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
  restorers.push(
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

/** ダイアログ（z-index 101）の中に popup を出す部品を置く。 */
function mountInDialog(inner: () => ReturnType<typeof h>): VueWrapper {
  return mount(
    { render: () => h(BaseDialog, { open: true, title: 'T' }, { default: inner }) },
    { attachTo: document.body },
  )
}

function zIndexOf(el: Element): number {
  return Number(getComputedStyle(el).zIndex)
}

/** ダイアログ本体の z-index。0 のまま比較が素通りしないよう 101 であることも確かめる。 */
function dialogZIndex(): number {
  const z = zIndexOf(document.querySelector('.dlg') as Element)
  expect(z).toBe(101)
  return z
}

describe('ダイアログ内のポップアップ（#98）', () => {
  test('BaseSelect: 開いた一覧がダイアログ（z-index 101）より前面', async () => {
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
