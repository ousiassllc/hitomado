// つまみの操作（ドラッグ・つまみの上のホイール）と、トラックの押下（1 画面ぶん動き、押し続けると繰り返す）。#378
// つまみは領域の兄弟要素なので、ネイティブのバーがやっていたことをここで代わりにする。
import { type AxisName, readAxis } from './dom'
import { type Axis, INSET, scrollDeltaFor, type Thumb } from './metrics'
import type { Clock, Presence } from './presence'

export interface Current {
  axis: Axis
  thumb: Thumb
}

export function bindThumbInput(
  scroller: HTMLElement,
  thumbs: Record<AxisName, HTMLElement>,
  current: Record<AxisName, Current | undefined>,
  presence: Presence,
  signal: AbortSignal,
): void {
  let drag: { axis: AxisName; pointer: number; scroll: number } | undefined
  const position = (axis: AxisName, ev: PointerEvent): number =>
    axis === 'y' ? ev.clientY : ev.clientX
  const end = (axis: AxisName): void => {
    if (drag?.axis !== axis) return
    drag = undefined
    thumbs[axis].removeAttribute('data-drag')
    presence.release('drag')
  }

  for (const axis of ['y', 'x'] as const) {
    const el = thumbs[axis]
    el.addEventListener(
      'pointerdown',
      (ev) => {
        const cur = current[axis]
        if (!cur || ev.button !== 0) return
        ev.preventDefault()
        el.setPointerCapture?.(ev.pointerId)
        drag = { axis, pointer: position(axis, ev), scroll: cur.axis.scroll }
        el.setAttribute('data-drag', '')
        presence.hold('drag')
      },
      { signal },
    )
    el.addEventListener(
      'pointermove',
      (ev) => {
        const cur = current[axis]
        if (drag?.axis !== axis || !cur) return
        const next =
          drag.scroll + scrollDeltaFor(cur.axis, cur.thumb, position(axis, ev) - drag.pointer)
        if (axis === 'y') scroller.scrollTop = next
        else scroller.scrollLeft = next
      },
      { signal },
    )
    for (const type of ['pointerup', 'pointercancel', 'lostpointercapture']) {
      el.addEventListener(type, () => end(axis), { signal })
    }
    // つまみの上のホイールは領域に渡す（渡さないと、つまみの上だけスクロールが止まる）。
    el.addEventListener(
      'wheel',
      (ev) => {
        const unit = ev.deltaMode === 1 ? 16 : ev.deltaMode === 2 ? scroller.clientHeight : 1
        scroller.scrollBy({ left: ev.deltaX * unit, top: ev.deltaY * unit })
        ev.preventDefault()
      },
      { signal, passive: false },
    )
  }
}

/** Chromium のネイティブのバーのトラック押下が動く量（表示の高さ・幅に対する割合）。 */
const PAGE_RATIO = 0.875
/** 押し続けたとき、最初の 1 画面から繰り返しに入るまでの待ち（ms）。 */
export const REPEAT_DELAY_MS = 500
/** 繰り返しの間隔（ms）。 */
export const REPEAT_INTERVAL_MS = 50

/**
 * 見えないトラックを押したら、つまみがポインタの手前なら進む向き・先なら戻る向きへ 1 画面ぶん動く。つまみの上はつまみ（ドラッグ）が受ける。
 * 押し続けると、待ってから一定の間隔で同じことを繰り返す。向きはそのたびにポインタの今の位置で決め、
 * つまみがポインタの位置に達したら（ポインタがつまみの上に来たら）動かさず、離すか端まで来たら止める。
 */
export function bindTrackInput(
  scroller: HTMLElement,
  tracks: Record<AxisName, HTMLElement>,
  current: Record<AxisName, Current | undefined>,
  clock: Clock,
  signal: AbortSignal,
): void {
  let timer: unknown
  const stop = (): void => {
    if (timer !== undefined) clock.clear(timer)
    timer = undefined
  }
  signal.addEventListener('abort', stop, { once: true })

  for (const axis of ['y', 'x'] as const) {
    const el = tracks[axis]
    let pointer: number | undefined // 押している間のポインタの位置（軸方向の clientY／clientX）
    const at = (): number | undefined => {
      if (pointer === undefined) return undefined
      const rect = el.getBoundingClientRect()
      return pointer - (axis === 'y' ? rect.top : rect.left)
    }
    /**
     * つまみがポインタの手前なら 1、先なら -1、ポインタの上（または端まで来た）なら 0。
     * つまみは描き直し（フレーム待ち）より先に動くので、位置は保存済みの値でなく今のスクロール位置から出す。
     */
    const direction = (): -1 | 0 | 1 => {
      const cur = current[axis]
      const pos = at()
      if (!cur || pos === undefined) return 0
      const now = readAxis(scroller, axis)
      const max = now.size - now.client
      const offset =
        INSET + cur.thumb.travel * Math.min(1, Math.max(0, max > 0 ? now.scroll / max : 0))
      const dir = pos < offset ? -1 : pos > offset + cur.thumb.length ? 1 : 0
      if ((dir < 0 && now.scroll <= 0) || (dir > 0 && now.scroll >= max)) return 0
      return dir
    }
    const page = (dir: -1 | 1): void => {
      const step = dir * (current[axis]?.axis.client ?? 0) * PAGE_RATIO
      scroller.scrollBy(
        axis === 'y' ? { top: step, behavior: 'instant' } : { left: step, behavior: 'instant' },
      )
    }
    const repeat = (): void => {
      const dir = direction()
      if (dir !== 0) page(dir)
      timer = clock.set(repeat, REPEAT_INTERVAL_MS)
    }
    const release = (): void => {
      pointer = undefined
      stop()
    }

    el.addEventListener(
      'pointerdown',
      (ev) => {
        if (!current[axis] || ev.button !== 0 || ev.target !== el) return
        ev.preventDefault()
        stop()
        pointer = axis === 'y' ? ev.clientY : ev.clientX
        const dir = direction()
        if (dir === 0) {
          pointer = undefined // つまみの上はつまみが受ける
          return
        }
        el.setPointerCapture?.(ev.pointerId)
        page(dir)
        timer = clock.set(repeat, REPEAT_DELAY_MS)
      },
      { signal },
    )
    el.addEventListener(
      'pointermove',
      (ev) => {
        if (pointer !== undefined) pointer = axis === 'y' ? ev.clientY : ev.clientX
      },
      { signal },
    )
    for (const type of ['pointerup', 'pointercancel', 'lostpointercapture']) {
      el.addEventListener(type, release, { signal })
    }
  }
}
