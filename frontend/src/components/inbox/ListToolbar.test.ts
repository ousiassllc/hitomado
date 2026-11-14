import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ListToolbar from './ListToolbar.vue'

function mountToolbar(
  searching: boolean,
  selectedCount = 0,
  compact = false,
  attachTo?: HTMLElement,
) {
  setActivePinia(createPinia())
  return mount(ListToolbar, {
    attachTo,
    props: {
      selectedCount,
      compact,
      refreshing: false,
      searching,
      unreadOnly: false,
      labelEnabled: true,
      labels: [],
      labelsLoading: false,
    },
  })
}

describe('ListToolbar の更新ボタン（#234）', () => {
  test('更新中は押せず、aria-busy でスピナーを出す（disabled の opacity は掛けない）', async () => {
    const w = mountToolbar(false)
    await w.setProps({ refreshing: true })
    const btn = w.get('button[aria-label="更新"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('aria-busy')).toBe('true')
    expect(btn.find('[role="status"]').exists()).toBe(true)
    w.unmount()
  })
})

describe('ListToolbar の検索結果モード', () => {
  test('検索語は繰り返さず、「受信箱に戻る」だけを出す（語は検索ボックスに残る）', async () => {
    const w = mountToolbar(true)
    expect(w.text()).not.toContain('検索結果')
    expect(w.text()).not.toContain('from:a@example.com')
    await w.get('button.back').trigger('click')
    expect(w.emitted('backToInbox')).toHaveLength(1)
    w.unmount()
  })

  test('検索中でなければ「受信箱に戻る」を出さない', () => {
    const w = mountToolbar(false)
    expect(w.find('button.back').exists()).toBe(false)
    w.unmount()
  })
})

// スレッドのヘッダーに同じ 4 操作があるので、一覧のツールバーは複数選択のときだけ出す。
describe('ListToolbar の一括操作（複数選択のときだけ）', () => {
  const ops = ['既読にする', '未読にする', 'アーカイブ', 'ゴミ箱へ移動', 'ラベル']
  const present = (w: ReturnType<typeof mountToolbar>) =>
    ops.filter((label) => w.find(`button[aria-label="${label}"]`).exists())

  test('0 件・1 件の選択では出さない（更新は残る）', () => {
    for (const n of [0, 1]) {
      const w = mountToolbar(false, n)
      expect(present(w)).toEqual([])
      expect(w.find('button[aria-label="更新"]').exists()).toBe(true)
      w.unmount()
    }
  })

  test('2 件以上の選択では 4 操作（既読・未読・アーカイブ・ゴミ箱・ラベル）を出し、押せる', async () => {
    const w = mountToolbar(false, 2)
    expect(present(w)).toEqual(ops)
    await w.get('button[aria-label="アーカイブ"]').trigger('click')
    await w.get('button[aria-label="ゴミ箱へ移動"]').trigger('click')
    await w.get('button[aria-label="未読にする"]').trigger('click')
    expect(w.emitted('archive')).toHaveLength(1)
    expect(w.emitted('trash')).toHaveLength(1)
    expect(w.emitted('toggleRead')).toEqual([[false]])
    expect(w.text()).toContain('2 件選択中')
    w.unmount()
  })

  test('1 画面ずつのレイアウト（compact）では、複数選択でも出さない', () => {
    const w = mountToolbar(false, 3, true)
    expect(present(w)).toEqual([])
    w.unmount()
  })
})

// Reka UI のトリガーは ArrowDown でもメニューを開く。開いた経路によらず、開いた時点でラベルの取得を求める（#338）。
describe('ListToolbar のラベルメニュー', () => {
  const open = async (w: ReturnType<typeof mountToolbar>, key: string) => {
    const trigger = w.get('button[aria-label="ラベル"]').element as HTMLElement
    trigger.focus()
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
    await flushPromises()
  }

  for (const key of ['ArrowDown', 'Enter', ' ']) {
    test(`${JSON.stringify(key)} で開いたとき openLabels を 1 回通知する`, async () => {
      document.body.innerHTML = ''
      const w = mountToolbar(false, 2, false, document.body)
      expect(w.emitted('openLabels')).toBeUndefined()
      await open(w, key)
      expect(document.querySelector('[role="menu"]')).not.toBeNull()
      expect(w.emitted('openLabels')).toHaveLength(1)
      w.unmount()
    })
  }

  test('クリックで開いたときも通知し、閉じるときは通知しない', async () => {
    document.body.innerHTML = ''
    const w = mountToolbar(false, 2, false, document.body)
    await w.get('button[aria-label="ラベル"]').trigger('click', { button: 0, ctrlKey: false })
    await flushPromises()
    expect(document.querySelector('[role="menu"]')).not.toBeNull()
    expect(w.emitted('openLabels')).toHaveLength(1)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(w.emitted('openLabels')).toHaveLength(1)
    w.unmount()
  })
})
