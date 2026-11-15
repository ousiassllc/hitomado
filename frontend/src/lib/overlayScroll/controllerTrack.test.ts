import { afterEach, describe, expect, test } from 'bun:test'
import { attachOverlayScroll } from './controller'
import { FLASH_MS, HIDE_DELAY_MS } from './presence'
import { fire, makeEnv, rootOf, scroller, shown } from './testSupport'
import { REPEAT_DELAY_MS, REPEAT_INTERVAL_MS } from './thumbInput'

// つまみの下の見えないトラック（#378）。押すと、つまみのある側と逆なら戻る向き・先なら進む向きへ、表示の 87.5%（Chromium のページ送り）動く。
// happy-dom にはレイアウトが無くトラックの矩形は 0 なので、clientY／clientX は「トラックの先頭からの位置」としてそのまま渡す。
type Scrolled = { top?: number; left?: number; behavior?: string }

/** scrollBy を記録する。 */
function record(el: HTMLElement): Scrolled[] {
  const calls: Scrolled[] = []
  el.scrollBy = ((o: Scrolled) => void calls.push(o)) as typeof el.scrollBy
  return calls
}

function press(track: Element, at: { clientY?: number; clientX?: number }, button = 0): Event {
  const ev = new Event('pointerdown', { cancelable: true })
  Object.assign(ev, { button, pointerId: 1, ...at })
  track.dispatchEvent(ev)
  return ev
}

const trackOf = (el: HTMLElement, axis: 'y' | 'x'): HTMLElement =>
  rootOf(el).querySelector(`.os-track-${axis}`) as HTMLElement

afterEach(() => {
  document.body.innerHTML = ''
})

describe('トラックの押下（1 画面ぶん動く）', () => {
  test('つまみより先を押すと進む向き、手前を押すと戻る向きへ、高さの 87.5% 動く。つまみの下に敷く', () => {
    const t = makeEnv()
    // つまみの長さ 248・位置 2 + 248 × (250/500) = 126〜374
    const el = scroller({ size: 1000, client: 500, scroll: 250 })
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    const track = trackOf(el, 'y')
    expect(track.hidden).toBe(false)
    expect(track.style.height).toBe('500px')
    const kids = [...rootOf(el).children]
    const thumb = rootOf(el).querySelector('.os-thumb-y') as Element
    expect(kids.indexOf(track)).toBeLessThan(kids.indexOf(thumb)) // つまみが後ろ（上）で、当たりを取る
    const calls = record(el)
    expect(press(track, { clientY: 450 }).defaultPrevented).toBe(true)
    press(track, { clientY: 50 })
    expect(calls).toEqual([
      { top: 500 * 0.875, behavior: 'instant' },
      { top: -500 * 0.875, behavior: 'instant' },
    ])
  })

  test('つまみの上の位置・主ボタン以外は何もしない', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500, scroll: 250 })
    attachOverlayScroll(el, {}, t.env)
    const calls = record(el)
    press(trackOf(el, 'y'), { clientY: 200 })
    const secondary = press(trackOf(el, 'y'), { clientY: 450 }, 2)
    expect(calls).toEqual([])
    expect(secondary.defaultPrevented).toBe(false)
  })

  test('はみ出していない軸のトラックは隠れていて、押しても動かない', () => {
    const t = makeEnv()
    const el = scroller({ size: 400, client: 400 })
    attachOverlayScroll(el, {}, t.env)
    const calls = record(el)
    expect(trackOf(el, 'y').hidden).toBe(true)
    press(trackOf(el, 'y'), { clientY: 300 })
    expect(calls).toEqual([])
  })

  test('横のトラックは幅の 87.5% を左右へ動く。縦横がそろうと、トラックも隅の 12px を空ける', () => {
    const t = makeEnv()
    const el = document.createElement('div')
    el.style.overflowX = 'auto'
    el.style.overflowY = 'auto'
    Object.defineProperties(el, {
      scrollHeight: { get: () => 800 },
      clientHeight: { get: () => 400 },
      scrollWidth: { get: () => 1000 },
      clientWidth: { get: () => 500 },
      scrollTop: { get: () => 0, set: () => {} },
      scrollLeft: { get: () => 0, set: () => {} },
    })
    document.body.append(el)
    attachOverlayScroll(el, {}, t.env)
    expect(trackOf(el, 'y').style.height).toBe(`${400 - 12}px`)
    expect(trackOf(el, 'x').style.width).toBe(`${500 - 12}px`)
    const calls = record(el)
    press(trackOf(el, 'x'), { clientX: 400 }) // つまみ（2〜244）より先
    expect(calls).toEqual([{ left: 500 * 0.875, behavior: 'instant' }])
  })

  test('トラックの上にマウスがある間も出し続ける', () => {
    const t = makeEnv()
    const el = scroller({ size: 1000, client: 500 })
    attachOverlayScroll(el, {}, t.env)
    t.advance(FLASH_MS)
    expect(shown(el)).toBe(false)
    const track = trackOf(el, 'y')
    fire(track, 'pointerenter')
    expect(shown(el)).toBe(true)
    t.advance(HIDE_DELAY_MS * 3)
    expect(shown(el)).toBe(true)
    fire(track, 'pointerleave')
    t.advance(HIDE_DELAY_MS)
    expect(shown(el)).toBe(false)
  })
})

// 押し続けると繰り返す（溝の長押し）。向きは今のポインタの位置で決め、つまみがポインタに達したら止め、離したら止める。
describe('トラックの長押し（連続ページ送り）', () => {
  /** scrollBy が実際に位置を動かし、scroll を流してつまみを描き直す（ブラウザと同じ順）。動いた量を記録する。 */
  function live(el: HTMLElement, t: ReturnType<typeof makeEnv>, max: number): number[] {
    const tops: number[] = []
    el.scrollBy = ((o: Scrolled) => {
      tops.push(o.top ?? 0)
      el.scrollTop = Math.max(0, Math.min(max, el.scrollTop + (o.top ?? 0)))
      fire(el, 'scroll')
      t.flush()
    }) as typeof el.scrollBy
    return tops
  }
  const setup = (size: number, scroll = 0) => {
    const t = makeEnv()
    const el = scroller({ size, client: 500, scroll })
    attachOverlayScroll(el, {}, t.env)
    t.flush()
    return { t, el, track: trackOf(el, 'y'), tops: live(el, t, size - 500) }
  }
  const move = (track: Element, clientY: number): void => {
    const ev = new Event('pointermove')
    Object.assign(ev, { clientY })
    track.dispatchEvent(ev)
  }
  const thumbRange = (el: HTMLElement): [number, number] => {
    const th = rootOf(el).querySelector('.os-thumb-y') as HTMLElement
    const top = Number.parseFloat(th.style.transform.replace(/[^0-9.]/g, ''))
    return [top, top + Number.parseFloat(th.style.height)]
  }

  test('押した直後に 1 画面。待ち時間が過ぎたら、その時点と以降の間隔ごとに 1 画面ずつ進む', () => {
    const { t, track, tops } = setup(5000)
    press(track, { clientY: 490 })
    expect(tops).toEqual([437.5])
    t.advance(REPEAT_DELAY_MS - 1)
    expect(tops).toHaveLength(1)
    t.advance(1)
    expect(tops).toHaveLength(2)
    t.advance(REPEAT_INTERVAL_MS * 3)
    expect(tops).toHaveLength(5)
  })

  test('つまみがポインタの位置に達したら止まる（つまみの上に来たら動かさない）', () => {
    // 全体 3000・表示 500。つまみの長さ 約 82.7・動ける長さ 約 413。1 画面ぶんでつまみは約 72px 動く。
    const { t, el, track, tops } = setup(3000)
    press(track, { clientY: 250 })
    t.advance(REPEAT_DELAY_MS + REPEAT_INTERVAL_MS * 30)
    expect(tops).toHaveLength(3) // つまみの位置 2→74→147→219（〜301）。250 が中に入って止まる
    const [from, to] = thumbRange(el)
    expect(from).toBeLessThanOrEqual(250)
    expect(to).toBeGreaterThanOrEqual(250)
    fire(track, 'pointerup')
  })

  test('つまみの描き直し（フレーム待ち）より先に動いても、今のスクロール位置で止まる', () => {
    const { t, el, track } = setup(3000)
    const tops: number[] = []
    el.scrollBy = ((o: Scrolled) => {
      tops.push(o.top ?? 0)
      el.scrollTop += o.top ?? 0 // scroll イベントも描き直しも流さない
    }) as typeof el.scrollBy
    press(track, { clientY: 250 })
    t.advance(REPEAT_DELAY_MS + REPEAT_INTERVAL_MS * 30)
    expect(tops).toHaveLength(3)
    fire(track, 'pointerup')
  })

  test('離したら動かさず、繰り返しのタイマーも残さない', () => {
    const { t, track, tops } = setup(5000)
    const idle = t.timers
    press(track, { clientY: 490 })
    t.advance(REPEAT_DELAY_MS + REPEAT_INTERVAL_MS * 2)
    const count = tops.length
    expect(count).toBe(4)
    fire(track, 'pointerup')
    expect(t.timers).toBe(idle)
    t.advance(REPEAT_INTERVAL_MS * 20)
    expect(tops).toHaveLength(count)
  })

  test('端まで来たら、押し続けても動かさない', () => {
    const { t, track, tops } = setup(1000)
    press(track, { clientY: 495 })
    t.advance(REPEAT_DELAY_MS + REPEAT_INTERVAL_MS * 10)
    expect(tops).toHaveLength(2) // 1 画面（437.5）→ 端（500）まで。以降は動かさない
    fire(track, 'pointerup')
  })

  test('向きはポインタの今の位置で決まる。つまみの手前へ動かすと、戻る向きになる', () => {
    const { t, track, tops } = setup(5000, 2000)
    press(track, { clientY: 490 })
    expect(tops).toEqual([437.5])
    move(track, 5)
    t.advance(REPEAT_DELAY_MS)
    expect(tops.at(-1)).toBe(-437.5)
    fire(track, 'pointerup')
  })

  test('つまみの上を押しても、繰り返しは始まらない', () => {
    const { t, track, tops } = setup(5000)
    press(track, { clientY: 10 })
    t.advance(REPEAT_DELAY_MS * 3)
    expect(tops).toEqual([])
  })

  test('破棄すると、押している途中でもタイマーが残らない', () => {
    const t = makeEnv()
    const el = scroller({ size: 5000, client: 500 })
    const h = attachOverlayScroll(el, {}, t.env)
    t.flush()
    live(el, t, 4500)
    press(trackOf(el, 'y'), { clientY: 490 })
    h.destroy()
    expect(t.timers).toBe(0)
  })
})
