// Ousiass デザインガイド準拠の回帰テスト（O3）。トーストの表示時間と置き換えを確かめる。
import { afterEach, describe, expect, jest, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, nextTick } from 'vue'
import ToastHost from '../../src/components/shell/ToastHost.vue'
import { useUiStore } from '../../src/stores/ui'

afterEach(() => {
  jest.useRealTimers()
  document.body.innerHTML = ''
})

// @vue/test-utils は Transition を既定でスタブにし、after-enter を呼ばない。表示時間は入場が終わってから数えるので、
// 入場の終わりをテストが決める Transition に差し替える（enterDone() が、今のトーストの after-enter になる）。
// スタブは受け取った mode・appear・after-enter の有無も引けるようにする（テンプレートの書き方・属性順には依存しない）。
let enterDone = (): void => {}
let transition = { mode: undefined as string | undefined, appear: false, hasAfterEnter: false }
const ControlledTransition = defineComponent({
  inheritAttrs: false,
  props: { mode: String, appear: Boolean },
  setup(props, { slots, attrs }) {
    enterDone = () => (attrs.onAfterEnter as () => void)()
    return () => {
      transition = {
        mode: props.mode,
        appear: props.appear,
        hasAfterEnter: typeof attrs.onAfterEnter === 'function',
      }
      return slots.default?.()
    }
  },
})
const controlledTransition = {
  attachTo: document.body,
  global: { stubs: { transition: ControlledTransition } },
}

describe('トーストの出入り（O3）', () => {
  test('置き換えは前のものの退出を待ち（out-in）、初回も入場を数える（appear）。表示時間は入場完了（after-enter）で数え始める', async () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    const w = mount(ToastHost, controlledTransition)
    ui.pushToast('success', '保存しました')
    await nextTick()
    expect(transition).toEqual({ mode: 'out-in', appear: true, hasAfterEnter: true })
    expect(w.find('.msg').text()).toBe('保存しました')
    w.unmount()
  })
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
    const w = mount(ToastHost, controlledTransition)
    ui.pushToast('success', '保存しました')
    await nextTick()
    expect(w.find('.toast').exists()).toBe(true)
    jest.advanceTimersByTime(10_000)
    expect(ui.toast).toBeDefined() // 入場が終わるまでは数えない
    enterDone()
    jest.advanceTimersByTime(2999)
    expect(ui.toast).toBeDefined()
    jest.advanceTimersByTime(1)
    expect(ui.toast).toBeUndefined()
    w.unmount()
  })

  test('置き換えでは前のタイマーを止め、新しいトーストは入場が終わってから 3 秒出る', async () => {
    jest.useFakeTimers()
    setActivePinia(createPinia())
    const ui = useUiStore()
    const w = mount(ToastHost, controlledTransition)
    ui.pushToast('success', 'A')
    await nextTick()
    enterDone()
    jest.advanceTimersByTime(2000)
    ui.pushToast('info', 'B')
    await nextTick()
    jest.advanceTimersByTime(10_000) // 前のトーストのタイマーは残らず、入場前の B も消えない
    expect(ui.toast).toMatchObject({ message: 'B' })
    enterDone()
    jest.advanceTimersByTime(2999)
    expect(ui.toast).toMatchObject({ message: 'B' })
    jest.advanceTimersByTime(1)
    expect(ui.toast).toBeUndefined()
    w.unmount()
  })

  test('スナックバー（元に戻す）はトーストと別の見た目で、アクション付きの 5 秒で消える', async () => {
    jest.useFakeTimers()
    setActivePinia(createPinia())
    const ui = useUiStore()
    const w = mount(ToastHost, controlledTransition)
    ui.pushSnackbar('アーカイブしました。', { label: '元に戻す', run: () => {} })
    await nextTick()
    enterDone()
    expect(w.find('.snackbar').exists()).toBe(true)
    expect(w.find('.toast').exists()).toBe(false)
    expect(w.find('[data-testid="snackbar-action"]').text()).toBe('元に戻す')
    jest.advanceTimersByTime(4999)
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
