// lib/overlayScroll のテストの共通部品（時計・フレームを手で進める環境、レイアウトの無い happy-dom で寸法を決めた領域）。
import type { Env } from './controller'

/** 手で進める時計と、手で流すフレーム。 */
export function makeEnv() {
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
export function scroller(opts: { size: number; client: number; scroll?: number }): HTMLElement {
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

export const rootOf = (el: HTMLElement): HTMLElement => {
  const r = el.nextElementSibling as HTMLElement | null
  if (!r?.classList.contains('os-root')) throw new Error('.os-root が無い')
  return r
}
export const shown = (el: HTMLElement): boolean => rootOf(el).hasAttribute('data-shown')
export const fire = (el: Element, type: string, init: object = {}): void => {
  el.dispatchEvent(new Event(type, init))
}
