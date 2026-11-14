// スクロール領域に重ねるスクロールバー（#378）の Vue への取り付け。アプリのスクロール領域はすべてこれを使う。
//   <div v-overlay-scroll class="list">                                   … 基本
//   <div v-overlay-scroll="{ fade: 'var(--surface)', flashOn: id }">      … 続きがある側の端のフェード・開いたとき 1 秒出す条件
// Fragment がルートのコンポーネント（reka の SelectViewport）には v- が使えないので、関数 ref の `overlayScrollRef` を使う。
import type { ComponentPublicInstance, Directive } from 'vue'
import { attachOverlayScroll, type OverlayHandle, type OverlayOptions } from './controller'

const handles = new WeakMap<Element, OverlayHandle>()

function attach(el: HTMLElement, options: OverlayOptions | undefined): void {
  el.setAttribute('data-overlay-scroll', '') // CSS がネイティブのバーを隠す目印
  handles.set(el, attachOverlayScroll(el, options ?? {}))
}

function detach(el: Element): void {
  handles.get(el)?.destroy()
  handles.delete(el)
  el.removeAttribute('data-overlay-scroll')
}

export const vOverlayScroll: Directive<HTMLElement, OverlayOptions | undefined> = {
  created: (el) => el.setAttribute('data-overlay-scroll', ''), // 描画前にネイティブのバーを隠し、幅が一瞬動くのを避ける
  mounted: (el, binding) => attach(el, binding.value),
  updated: (el, binding) => handles.get(el)?.setOptions(binding.value ?? {}),
  unmounted: (el) => detach(el),
}

/** 関数 ref の取り付け先の要素。コンポーネントの `$el` が、Fragment の先頭の目印（テキスト・コメント）のときは、その次の要素。 */
function elementOf(target: Element | ComponentPublicInstance | null): Element | null {
  const el: unknown = target instanceof Element ? target : (target?.$el ?? null)
  if (el instanceof Element) return el
  if (!(el instanceof Node)) return null
  const next = el.nextSibling
  return next?.nodeType === 1 ? (next as Element) : null
}

/**
 * 関数 ref 版。`:ref="overlayScrollRef()"` のように、コンポーネントの script で 1 回作って渡す。外れるときは前の要素から外す。
 * `inner` は、コンポーネントの `$el` が class を付けた本体の外側の包み（reka の Popper の `data-reka-popper-content-wrapper`）のとき、
 * 本体を探すセレクタ（例 `.menu-pop`）。
 */
export function overlayScrollRef(
  options?: OverlayOptions,
  inner?: string,
): (target: Element | ComponentPublicInstance | null) => void {
  let attached: HTMLElement | undefined
  return (target) => {
    const base = elementOf(target)
    const el = inner && base ? (base.matches(inner) ? base : base.querySelector(inner)) : base
    const next = el instanceof HTMLElement ? el : undefined
    if (next === attached) return
    if (attached) detach(attached)
    attached = next
    if (next) attach(next, options)
  }
}
