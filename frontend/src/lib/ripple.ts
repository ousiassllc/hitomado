// ボタンとクローズ（アイコンボタン）の押下リップル（ガイド M4）。クリック点から円が広がって消える（280ms ease-out）。
// 動きは styles/reset.css の .ripple。prefers-reduced-motion では出さない（同ファイル）。
import type { Directive } from 'vue'

function spawn(el: HTMLElement, ev: PointerEvent): void {
  if (el instanceof HTMLButtonElement && el.disabled) return
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
  const rect = el.getBoundingClientRect()
  // クリック点から最も遠い角まで広げれば、ボタン全体を覆える。
  const radius = Math.hypot(
    Math.max(ev.clientX - rect.left, rect.right - ev.clientX),
    Math.max(ev.clientY - rect.top, rect.bottom - ev.clientY),
  )
  const dot = document.createElement('span')
  dot.className = 'ripple'
  dot.setAttribute('aria-hidden', 'true')
  dot.style.width = dot.style.height = `${radius * 2}px`
  dot.style.left = `${ev.clientX - rect.left - radius}px`
  dot.style.top = `${ev.clientY - rect.top - radius}px`
  dot.addEventListener('animationend', () => dot.remove(), { once: true })
  el.append(dot)
}

type RippleEl = HTMLElement & { __ripple?: (ev: PointerEvent) => void }

/** `v-ripple`。要素を position: relative・overflow: hidden にして（.has-ripple）、押下点にリップルを出す。 */
export const vRipple: Directive<HTMLElement> = {
  mounted(el: RippleEl) {
    el.classList.add('has-ripple')
    el.__ripple = (ev) => spawn(el, ev)
    el.addEventListener('pointerdown', el.__ripple)
  },
  unmounted(el: RippleEl) {
    if (el.__ripple) el.removeEventListener('pointerdown', el.__ripple)
  },
}
