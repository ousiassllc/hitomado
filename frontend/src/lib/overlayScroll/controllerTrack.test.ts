import { afterEach, describe, expect, test } from 'bun:test'
import { attachOverlayScroll } from './controller'
import { FLASH_MS, HIDE_DELAY_MS } from './presence'
import { fire, makeEnv, rootOf, scroller, shown } from './testSupport'

// つまみの下の見えないトラック（#378）。押すと、つまみのある側と逆なら戻る向き・先なら進む向きへ、表示の 87.5%（Chromium のページ送り）動く。
// happy-dom にはレイアウトが無くトラックの矩形は 0 なので、clientY／clientX は「トラックの先頭からの位置」としてそのまま渡す。
type Scrolled = { top?: number; left?: number }

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
    expect(calls).toEqual([{ top: 500 * 0.875 }, { top: -500 * 0.875 }])
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
    expect(calls).toEqual([{ left: 500 * 0.875 }])
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
