// controller.ts が使う DOM の小さな補助（#378）。
import type { Axis } from './metrics'

export type AxisName = 'y' | 'x'

export function create(doc: Document, className: string): HTMLElement {
  const el = doc.createElement('div')
  el.className = className
  return el
}

export function readAxis(s: HTMLElement, axis: AxisName): Axis {
  return axis === 'y'
    ? { scroll: s.scrollTop, size: s.scrollHeight, client: s.clientHeight }
    : { scroll: s.scrollLeft, size: s.scrollWidth, client: s.clientWidth }
}

/** ユーザーがその向きにスクロールできる領域か。文書（viewport）の overflow: visible はスクロールできる扱い。 */
export function scrollable(cs: CSSStyleDeclaration, axis: AxisName, viewport: boolean): boolean {
  const value = axis === 'y' ? cs.overflowY : cs.overflowX
  return value === 'auto' || value === 'scroll' || (viewport && value === 'visible')
}

/** キーボード操作のフォーカスか（マウスのクリックで当たったフォーカスでは出しっぱなしにしない）。 */
export function focusVisible(target: EventTarget | null): boolean {
  try {
    return (target as Element).matches(':focus-visible')
  } catch {
    return true
  }
}
