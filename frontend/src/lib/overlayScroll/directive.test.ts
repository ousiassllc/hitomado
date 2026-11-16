import { afterEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref, withDirectives } from 'vue'
import { overlayScrollRef, vOverlayScroll } from '.'

afterEach(() => {
  document.body.innerHTML = ''
})

const rootCount = (): number => document.querySelectorAll('.os-root').length

describe('v-overlay-scroll', () => {
  test('描画前にネイティブのバーを隠す目印を付け、マウント後に .os-root を兄弟に置く。アンマウントで外す', async () => {
    const Comp = defineComponent({
      props: { on: { type: Boolean, default: true } },
      setup: (props) => () =>
        h(
          'section',
          props.on ? [withDirectives(h('div', { class: 'area' }), [[vOverlayScroll]])] : [],
        ),
    })
    const w = mount(Comp, { attachTo: document.body })
    const area = w.get('.area').element
    expect(area.hasAttribute('data-overlay-scroll')).toBe(true)
    expect(area.nextElementSibling?.classList.contains('os-root')).toBe(true)
    expect(rootCount()).toBe(1)
    await w.setProps({ on: false })
    expect(rootCount()).toBe(0)
    w.unmount()
  })

  test('バインドの値が変わると setOptions に渡る（フェードの地の色）', async () => {
    const fade = ref<string | undefined>(undefined)
    const Comp = defineComponent({
      setup: () => () =>
        h('section', [
          withDirectives(h('div', { class: 'area' }), [[vOverlayScroll, { fade: fade.value }]]),
        ]),
    })
    const w = mount(Comp, { attachTo: document.body })
    const root = w.get('.os-root').element as HTMLElement
    expect(root.style.getPropertyValue('--os-fade-bg')).toBe('')
    fade.value = 'var(--surface)'
    await nextTick()
    expect(root.style.getPropertyValue('--os-fade-bg')).toBe('var(--surface)')
    w.unmount()
  })
})

describe('overlayScrollRef', () => {
  test('要素・コンポーネントの両方に取り付け、外れる（null）ときと付け替えで前の取り付けを解放する', () => {
    const setRef = overlayScrollRef()
    const a = document.createElement('div')
    const b = document.createElement('div')
    document.body.append(a, b)
    setRef(a)
    expect(a.hasAttribute('data-overlay-scroll')).toBe(true)
    expect(rootCount()).toBe(1)
    setRef(a) // 同じ要素の再代入（描画のたびに呼ばれる）では取り付け直さない
    expect(rootCount()).toBe(1)
    setRef({ $el: b } as never)
    expect(a.hasAttribute('data-overlay-scroll')).toBe(false)
    expect(rootCount()).toBe(1)
    setRef(null)
    expect(b.hasAttribute('data-overlay-scroll')).toBe(false)
    expect(rootCount()).toBe(0)
  })
})
