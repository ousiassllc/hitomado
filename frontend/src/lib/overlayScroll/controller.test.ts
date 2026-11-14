import { afterEach, describe, expect, test } from 'bun:test'
import { attachOverlayScroll, type Env } from './controller'
import { FLASH_MS, HIDE_DELAY_MS } from './presence'

/** 手で進める時計と、手で流すフレーム。 */
function makeEnv() {
  let now = 0
  let nextId = 1
  const timers = new Map<number, { at: number; fn: () => void }>()
  const frames = new Map<number, () => void>()
  const env: Env = {
    clock: {
      set: (fn, ms) => {
        const id = nextId++
        timers.set(id, { at: now + ms, fn })
        return id
      },
      clear: (h) => void timers.delete(h as number),
    },
    frame: {
      request: (fn) => {
        const id = nextId++
        frames.set(id, fn)
        return id
      },
      cancel: (h) => void frames.delete(h as number),
    },
  }
  return {
    env,
    get timers() {
      return timers.size
    },
    get frames() {
      return frames.size
    },
    flush() {
      const fns = [...frames.values()]
      frames.clear()
      for (const fn of fns) fn()
    },
    advance(ms: number) {
      const end = now + ms
      for (;;) {
        const due = [...timers.entries()]
          .filter(([, t]) => t.at <= end)
          .sort((a, b) => a[1].at - b[1].at)[0]
        if (!due) break
        timers.delete(due[0])
        now = due[1].at
        due[1].fn()
      }
      now = end
    },
  }
}

/** happy-dom にはレイアウトが無いので、寸法は自分で決める。 */
function scroller(opts: { size: number; client: number; scroll?: number }): HTMLElement {
  const el = document.createElement('div')
  el.style.overflowY = 'auto'
  let scrollTop = opts.scroll ?? 0
  Object.defineProperties(el, {
    scrollHeight: { get: () => opts.size },
    clientHeight: { get: () => opts.client },
    clientWidth: { get: () => 300 },
    scrollWidth: { get: () => 300 },
    scrollTop: {
      get: () => scrollTop,
      set: (v: number) => {
        scrollTop = v
      },
    },
  })
  document.body.append(el)
  return el
}

const rootOf = (el: HTMLElement): HTMLElement => {
  const r = el.nextElementSibling as HTMLElement | null
  if (!r?.classList.contains('os-root')) throw new Error('.os-root が無い')
  return r
}
const shown = (el: HTMLElement): boolean => rootOf(el).hasAttribute('data-shown')
const fire = (el: Element, type: string, init: object = {}): void => {
  el.dispatchEvent(new Event(type, init))
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('attachOverlayScroll', () => {
  test('スクロール領域の次の兄弟に .os-root を置き、つまみの寸法を入れる（幅を取らない）', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    const h = attachOverlayScroll(el, {}, t.env)
    const thumb = rootOf(el).querySelector<HTMLElement>('.os-thumb-y')
    expect(thumb?.hidden).toBe(false)
    expect(thumb?.style.height).toBe(`${(500 - 4) / 2}px`)
    expect(thumb?.style.transform).toBe('translateY(2px)')
    expect(el.clientWidth).toBe(300) // 領域の幅は変わらない（隠すのも重ねるのも CSS と兄弟要素だけ）
    h.destroy()
  })

  test('はみ出していなければつまみを出さず、予告もしない', () => {
    const t = makeEnv()
    const el = scroller({ size: 400, client: 400 })
    attachOverlayScroll(el, {}, t.env)
    expect(rootOf(el).querySelector<HTMLElement>('.os-thumb-y')?.hidden).toBe(true)
    expect(shown(el)).toBe(false)
    expect(t.timers).toBe(0)
  })

  test('開いたとき、はみ出していれば 1 秒だけ出して消す', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    expect(shown(el)).toBe(true)
    t.advance(FLASH_MS - 1)
    expect(shown(el)).toBe(true)
    t.advance(1)
    expect(shown(el)).toBe(false)
  })

  test('マウスが乗ると出て、外れて 800ms 後に消える。つまみの上に移っても外に出たことにしない', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    fire(el, 'pointerenter')
    expect(shown(el)).toBe(true)
    const thumb = rootOf(el).querySelector('.os-thumb-y') as HTMLElement
    // 領域 → つまみ（兄弟要素）へ: leave のあとすぐ enter
    fire(el, 'pointerleave')
    fire(thumb, 'pointerenter')
    t.advance(HIDE_DELAY_MS * 2)
    expect(shown(el)).toBe(true)
    fire(thumb, 'pointerleave')
    t.advance(HIDE_DELAY_MS - 1)
    expect(shown(el)).toBe(true)
    t.advance(1)
    expect(shown(el)).toBe(false)
  })

  test('タッチはホバーで出さない。スクロールしている間だけ出して、止まって 800ms 後に消す', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    const ev = new Event('pointerenter')
    Object.defineProperty(ev, 'pointerType', { value: 'touch' })
    el.dispatchEvent(ev)
    expect(shown(el)).toBe(false)
    fire(el, 'scroll')
    expect(shown(el)).toBe(true)
    t.advance(HIDE_DELAY_MS)
    expect(shown(el)).toBe(false)
  })

  test('フォーカスが中にある間は出し続ける（外へ移ったら 800ms 後に消す）', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    const input = document.createElement('button')
    input.matches = (q: string) => q === ':focus-visible' // happy-dom は :focus-visible を判定しない（キーボードで入ったフォーカスとして扱う）
    el.append(input)
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    fire(input, 'focusin', { bubbles: true })
    t.advance(HIDE_DELAY_MS * 3)
    expect(shown(el)).toBe(true)
    fire(input, 'focusout', { bubbles: true })
    t.advance(HIDE_DELAY_MS)
    expect(shown(el)).toBe(false)
  })

  test('つまみのドラッグ中は、マウスが外へ出ても消さない。離すと 800ms 後に消す。ドラッグでスクロールが動く', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    const thumb = rootOf(el).querySelector('.os-thumb-y') as HTMLElement
    const down = new Event('pointerdown', { cancelable: true })
    Object.assign(down, { button: 0, pointerId: 1, clientY: 100 })
    thumb.dispatchEvent(down)
    expect(thumb.hasAttribute('data-drag')).toBe(true)
    const move = new Event('pointermove')
    Object.assign(move, { pointerId: 1, clientY: 100 + 50 })
    thumb.dispatchEvent(move)
    // つまみが動ける長さ 248（トラック 496 − つまみ 248）に対しスクロールの範囲は 500 → 50px 動かすと 50 × 500 / 248
    expect(el.scrollTop).toBeCloseTo((50 * 500) / 248, 3)
    t.advance(HIDE_DELAY_MS * 3)
    expect(shown(el)).toBe(true)
    fire(thumb, 'pointerup')
    expect(thumb.hasAttribute('data-drag')).toBe(false)
    t.advance(HIDE_DELAY_MS)
    expect(shown(el)).toBe(false)
  })

  test('続きがある側にだけフェードを付ける（端まで来たらその側は消える）。fade を渡さなければ置かない', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    const h = attachOverlayScroll(el, { fade: 'var(--surface)' }, t.env)
    const root = rootOf(el)
    expect(root.style.getPropertyValue('--os-fade-bg')).toBe('var(--surface)')
    expect(root.hasAttribute('data-fade-top')).toBe(false)
    expect(root.hasAttribute('data-fade-bottom')).toBe(true)
    el.scrollTop = 200
    fire(el, 'scroll')
    t.flush()
    expect(root.hasAttribute('data-fade-top')).toBe(true)
    expect(root.hasAttribute('data-fade-bottom')).toBe(true)
    el.scrollTop = 500
    fire(el, 'scroll')
    t.flush()
    expect(root.hasAttribute('data-fade-top')).toBe(true)
    expect(root.hasAttribute('data-fade-bottom')).toBe(false)
    h.destroy()

    const plain = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(plain, {}, t.env)
    expect(rootOf(plain).hasAttribute('data-fade-bottom')).toBe(false)
  })

  test('flashOn が変わったら、はみ出していれば 1 秒出す。はみ出していなければ出さない', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    const h = attachOverlayScroll(el, { flashOn: 'a' }, t.env)
    t.advance(FLASH_MS)
    expect(shown(el)).toBe(false)
    h.setOptions({ flashOn: 'a' })
    expect(shown(el)).toBe(false)
    h.setOptions({ flashOn: 'b' })
    expect(shown(el)).toBe(true)
    t.advance(FLASH_MS)
    expect(shown(el)).toBe(false)

    const small = scroller({ size: 300, client: 500 })
    const h2 = attachOverlayScroll(small, { flashOn: 'a' }, t.env)
    h2.setOptions({ flashOn: 'b' })
    expect(shown(small)).toBe(false)
  })

  test('destroy で .os-root・タイマー・フレームを全部解放し、以後のイベントに反応しない', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    const h = attachOverlayScroll(el, {}, t.env)
    fire(el, 'scroll') // タイマー（800ms）とフレームを握った状態で
    expect(t.timers).toBe(1)
    expect(t.frames).toBe(1)
    h.destroy()
    expect(el.nextElementSibling).toBeNull()
    expect(t.timers).toBe(0)
    expect(t.frames).toBe(0)
    fire(el, 'scroll')
    fire(el, 'pointerenter')
    expect(t.timers).toBe(0)
    expect(t.frames).toBe(0)
  })

  test('領域が隠れている間は重ねる要素も隠す', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    const root = rootOf(el)
    expect(root.hidden).toBe(false)
    el.getClientRects = () => [] as unknown as DOMRectList
    fire(el, 'scroll')
    t.flush()
    expect(root.hidden).toBe(true)
  })
})
