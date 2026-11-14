// つまみの操作（ドラッグ・つまみの上のホイール。#378）。つまみは領域の兄弟要素なので、ネイティブのバーが
// やっていたことをここで代わりにする。
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
