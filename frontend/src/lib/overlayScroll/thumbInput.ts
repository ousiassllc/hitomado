// つまみの操作（ドラッグ・つまみの上のホイール）と、トラックの押下（1 画面ぶん動く）。#378
// つまみは領域の兄弟要素なので、ネイティブのバーがやっていたことをここで代わりにする。
import type { AxisName } from './dom'
import { type Axis, scrollDeltaFor, type Thumb } from './metrics'
import type { Presence } from './presence'

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

/** 見えないトラックを押したら、つまみのある側と逆なら戻る向き・先なら進む向きへ 1 画面ぶん動く。つまみの上はつまみ（ドラッグ）が受ける。 */
export function bindTrackInput(
  scroller: HTMLElement,
  tracks: Record<AxisName, HTMLElement>,
  current: Record<AxisName, Current | undefined>,
  signal: AbortSignal,
): void {
  for (const axis of ['y', 'x'] as const) {
    const el = tracks[axis]
    el.addEventListener(
      'pointerdown',
      (ev) => {
        const cur = current[axis]
        if (!cur || ev.button !== 0 || ev.target !== el) return
        ev.preventDefault()
        const rect = el.getBoundingClientRect()
        const at = axis === 'y' ? ev.clientY - rect.top : ev.clientX - rect.left
        const { offset, length } = cur.thumb
        if (at >= offset && at <= offset + length) return
        const step = (at < offset ? -1 : 1) * cur.axis.client * PAGE_RATIO
        scroller.scrollBy(axis === 'y' ? { top: step } : { left: step })
      },
      { signal },
    )
  }
}
