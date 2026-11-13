// NF-P04: 受信箱リストの仮想スクロール。2,000 行を渡しても、見える範囲＋overscan の行だけを DOM に出す。
import { afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import type { ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import ThreadList from './ThreadList.vue'

setActivePinia(createPinia())

const TOTAL = 2000

const items: ThreadSummary[] = Array.from({ length: TOTAL }, (_, i) => ({
  accountId: 'a1',
  threadId: `t${i}`,
  subject: `件名 ${i}`,
  snippet: 'お世話になっております',
  lastMessageAt: Date.now() - i * 1000,
  lastFrom: { name: '山田太郎', email: 'yamada@example.com' },
  messageCount: 1,
  isUnread: false,
  hasAttachment: false,
  inInbox: true,
  labels: [],
}))

const props = {
  items,
  checkedKeys: [],
  loading: false,
  hasMore: false,
  loadingMore: false,
  emptyText: '空',
}

// 仮想スクロールはスクロール領域の高さが 0 だと行を描かないので、高さを持たせる（threadListMore.test.ts と同じ）。
const descriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight')
// scrollToIndex は scrollTo を呼ぶ。happy-dom の scrollTo は scrollTop も scroll イベントも動かさないので、実ブラウザ相当に補う。
const originalScrollTo = HTMLElement.prototype.scrollTo
// virtual-core は scrollHeight − clientHeight でスクロールの上限を決める。happy-dom の scrollHeight は 0 なので、内容の高さ（.inner の style.height）に補う。
const scrollHeightDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'scrollHeight',
)
beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
    configurable: true,
    get(this: HTMLElement) {
      return Number.parseFloat(this.querySelector<HTMLElement>('.inner')?.style.height ?? '0')
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, value: 600 })
  HTMLElement.prototype.scrollTo = function (this: HTMLElement, opts?: ScrollToOptions | number) {
    this.scrollTop = typeof opts === 'object' ? (opts.top ?? 0) : (opts ?? 0)
    this.dispatchEvent(new Event('scroll'))
  } as typeof HTMLElement.prototype.scrollTo
})
afterEach(() => {
  HTMLElement.prototype.scrollTo = originalScrollTo
  if (scrollHeightDescriptor)
    Object.defineProperty(HTMLElement.prototype, 'scrollHeight', scrollHeightDescriptor)
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollHeight')
  if (descriptor) Object.defineProperty(HTMLElement.prototype, 'clientHeight', descriptor)
  else Reflect.deleteProperty(HTMLElement.prototype, 'clientHeight')
})

function posinsets(w: ReturnType<typeof mount>): number[] {
  return w.findAll('[role="option"]').map((o) => Number(o.attributes('aria-posinset')))
}

describe('ThreadList の仮想スクロール（NF-P04）', () => {
  test('2,000 行を渡しても、描画する option は見える範囲＋overscan だけ（100 未満）', async () => {
    const w = mount(ThreadList, { props })
    await nextTick()
    const count = w.findAll('[role="option"]').length
    expect(count).toBeGreaterThan(0)
    expect(count).toBeLessThan(100)
  })

  test('各 option の aria-posinset・aria-setsize は 2,000 件基準の位置のまま', async () => {
    const w = mount(ThreadList, { props })
    await nextTick()
    const options = w.findAll('[role="option"]')
    expect(posinsets(w)).toEqual(options.map((_, i) => i + 1))
    expect(options.every((o) => o.attributes('aria-setsize') === String(TOTAL))).toBe(true)
  })

  test('cursorKey を末尾の行にすると、その行が描画対象になる', async () => {
    const w = mount(ThreadList, { props })
    await nextTick()
    expect(posinsets(w)).not.toContain(TOTAL)
    await w.setProps({ cursorKey: `a1:t${TOTAL - 1}` })
    // watch 内の nextTick の後に scrollToIndex が呼ばれ、描画範囲の更新が続くので、上限付きで待つ。
    for (let i = 0; i < 50 && !posinsets(w).includes(TOTAL); i++) {
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
    expect(posinsets(w)).toContain(TOTAL)
    expect(w.findAll('[role="option"]').length).toBeLessThan(100)
  })
})
