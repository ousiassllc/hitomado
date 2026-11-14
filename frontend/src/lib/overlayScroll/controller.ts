// スクロールバーを「重ねて出す」ためのコントローラ（#378）。
// ネイティブのバーは scrollbar-width: none で隠し（styles/overlayScroll.css）、同じ寸法をこの要素が描く。領域の幅は動かない。
//   - 描く要素（.os-root）は、スクロール領域の次の兄弟として入れ、領域と同じ大きさの `position: absolute` で重ねる。
//     置く位置は getBoundingClientRect の差から決める（親が position していなくても、transform されていても合う）。
//   - 出入りは presence.ts。つまみの位置・長さは metrics.ts。見た目と時間（フェード・幅の変化）は CSS。
//   - 文書のスクロール（本文の iframe の中。viewport: true）は、documentElement を領域にして `position: fixed` で重ねる。
// 取り付けたリスナー・オブザーバー・タイマー・rAF は destroy で全部解放する。
import { type AxisName, create, focusVisible, readAxis, scrollable } from './dom'
import { type Axis, CORNER, hasMore, overflows, type Thumb, thumbFor } from './metrics'
import { browserClock, type Clock, Presence } from './presence'
import { bindThumbInput, type Current } from './thumbInput'

export interface OverlayOptions {
  /** 続きがある側（上・下）の端に 16px のフェードを置くときの地の色（CSS の色。例 `var(--surface)`）。無ければ置かない。 */
  fade?: string
  /** 変わったら、はみ出していればスクロールバーを 1 秒出す（別のメールを開いた・設定のタブを切り替えた）。 */
  flashOn?: unknown
}

export interface Env {
  clock: Clock
  frame: { request(fn: () => void): unknown; cancel(handle: unknown): void }
}

export const browserEnv: Env = {
  clock: browserClock,
  frame: {
    request: (fn) => requestAnimationFrame(fn),
    cancel: (handle) => cancelAnimationFrame(handle as number),
  },
}

export interface OverlayHandle {
  setOptions(next: OverlayOptions): void
  destroy(): void
}

export function attachOverlayScroll(
  scroller: HTMLElement,
  initial: OverlayOptions,
  env: Env = browserEnv,
): OverlayHandle {
  const doc = scroller.ownerDocument
  const viewport = scroller === doc.documentElement
  const abort = new AbortController()
  const { signal } = abort
  let options: OverlayOptions = {}

  const root = create(doc, viewport ? 'os-root os-viewport' : 'os-root')
  root.setAttribute('aria-hidden', 'true')
  const fadeTop = create(doc, 'os-fade os-fade-top')
  const fadeBottom = create(doc, 'os-fade os-fade-bottom')
  const thumbs: Record<AxisName, HTMLElement> = {
    y: create(doc, 'os-thumb os-thumb-y'),
    x: create(doc, 'os-thumb os-thumb-x'),
  }
  for (const t of Object.values(thumbs)) t.hidden = true
  root.append(fadeTop, fadeBottom, thumbs.y, thumbs.x)
  if (viewport) doc.body.append(root)
  else scroller.after(root)

  const presence = new Presence(env.clock, (shown) => root.toggleAttribute('data-shown', shown))

  // --- 計測と反映（rAF で 1 フレームに 1 回） ---
  let pending: unknown
  let hadOverflow = false
  let placed = { left: 0, top: 0 }
  const current: Record<AxisName, Current | undefined> = { y: undefined, x: undefined }

  function measure(): void {
    // 領域が隠れている（display: none・v-show）間は重ねる要素も隠す。隠れていなければ、CSS が描かない場面（Android のタッチ UI・ハイコントラスト）かを見る。
    root.hidden = scroller.getClientRects().length === 0
    if (root.hidden) hadOverflow = false // 隠れていたものが出てきたら、はみ出していれば改めて予告する
    if (root.hidden || root.getClientRects().length === 0) return
    const cs = doc.defaultView?.getComputedStyle(scroller)
    if (!cs) return
    const ay = scrollable(cs, 'y', viewport) ? readAxis(scroller, 'y') : undefined
    const ax = scrollable(cs, 'x', viewport) ? readAxis(scroller, 'x') : undefined
    const overY = ay !== undefined && overflows(ay)
    const overX = ax !== undefined && overflows(ax)

    // 領域の位置。同じ親の中なので、offsetLeft／offsetTop（レイアウトの位置。領域自身の transform やアニメーションの途中の動きを含まない）で足りる。
    // 基準になる要素（offsetParent）が違うとき（親が static で body の余白がある等）だけ、実測の差で割り出す。
    let left = 0
    let top = 0
    if (!viewport) {
      if (scroller.offsetParent === root.offsetParent) {
        left = scroller.offsetLeft + scroller.clientLeft
        top = scroller.offsetTop + scroller.clientTop
      } else {
        const s = scroller.getBoundingClientRect()
        const r = root.getBoundingClientRect()
        left = s.left + scroller.clientLeft - (r.left - placed.left)
        top = s.top + scroller.clientTop - (r.top - placed.top)
      }
    }
    placed = { left, top }
    root.style.left = `${left}px`
    root.style.top = `${top}px`
    root.style.width = `${scroller.clientWidth}px`
    root.style.height = `${scroller.clientHeight}px`

    show('y', ay, overY && ay ? thumbFor(ay, overX ? CORNER : 0) : null)
    show('x', ax, overX && ax ? thumbFor(ax, overY ? CORNER : 0) : null)

    const more = options.fade !== undefined && ay ? hasMore(ay) : { before: false, after: false }
    root.toggleAttribute('data-fade-top', more.before)
    root.toggleAttribute('data-fade-bottom', more.after)

    const overflowing = overY || overX
    if (overflowing && !hadOverflow) presence.flash() // はみ出しているエリアが開いた
    hadOverflow = overflowing
  }

  function show(axis: AxisName, a: Axis | undefined, t: Thumb | null): void {
    const el = thumbs[axis]
    el.hidden = t === null
    current[axis] = t && a ? { axis: a, thumb: t } : undefined
    if (!t) return
    if (axis === 'y') {
      el.style.height = `${t.length}px`
      el.style.transform = `translateY(${t.offset}px)`
    } else {
      el.style.width = `${t.length}px`
      el.style.transform = `translateX(${t.offset}px)`
    }
  }

  function schedule(): void {
    if (pending !== undefined) return
    pending = env.frame.request(() => {
      pending = undefined
      measure()
    })
  }

  // --- 出入り ---
  const scrollTarget: EventTarget = viewport ? doc : scroller
  scrollTarget.addEventListener(
    'scroll',
    () => {
      presence.activity()
      schedule()
    },
    { signal, passive: true },
  )

  // textarea は中身が変わっても大きさが変わらない（ResizeObserver が拾えない）ので、入力のたびに測り直す。
  scroller.addEventListener('input', schedule, { signal })

  const hovering = new Set<EventTarget>()
  const listenHover = (el: HTMLElement): void => {
    el.addEventListener(
      'pointerenter',
      (ev) => {
        if ((ev as PointerEvent).pointerType === 'touch') return // タッチはスクロール中だけ出す
        hovering.add(el)
        presence.hold('hover')
        schedule()
      },
      { signal },
    )
    el.addEventListener(
      'pointerleave',
      () => {
        if (hovering.delete(el) && hovering.size === 0) presence.release('hover')
      },
      { signal },
    )
  }
  listenHover(scroller)
  listenHover(thumbs.y)
  listenHover(thumbs.x)

  scrollTarget.addEventListener(
    'focusin',
    (ev) => {
      if (!focusVisible(ev.target)) return
      presence.hold('focus')
      schedule()
    },
    { signal },
  )
  scrollTarget.addEventListener(
    'focusout',
    (ev) => {
      const next = (ev as FocusEvent).relatedTarget as Node | null
      if (next && scroller.contains(next)) return
      presence.release('focus')
    },
    { signal },
  )

  bindThumbInput(scroller, thumbs, current, presence, signal)

  // --- 大きさの変化（内容が増えた・領域が伸びた） ---
  const Resize = doc.defaultView?.ResizeObserver
  const Mutation = doc.defaultView?.MutationObserver
  const resize = Resize ? new Resize(schedule) : undefined
  const watchTargets = (): void => {
    resize?.disconnect()
    if (viewport) {
      resize?.observe(doc.documentElement)
      resize?.observe(doc.body)
      return
    }
    resize?.observe(scroller)
    for (const child of Array.from(scroller.children)) resize?.observe(child)
  }
  watchTargets()
  const mutation =
    Mutation && !viewport
      ? new Mutation(() => {
          watchTargets()
          schedule()
        })
      : undefined
  mutation?.observe(scroller, { childList: true })

  function setOptions(next: OverlayOptions): void {
    const flash = !Object.is(next.flashOn, options.flashOn)
    const fadeChanged = next.fade !== options.fade
    options = { fade: next.fade, flashOn: next.flashOn }
    if (fadeChanged) {
      if (next.fade === undefined) root.style.removeProperty('--os-fade-bg')
      else root.style.setProperty('--os-fade-bg', next.fade)
    }
    if (flash && initialised) {
      measure()
      if (hadOverflow) presence.flash()
    } else if (fadeChanged) schedule()
  }

  let initialised = false
  setOptions(initial)
  initialised = true
  measure()

  return {
    setOptions,
    destroy() {
      abort.abort()
      resize?.disconnect()
      mutation?.disconnect()
      presence.destroy()
      if (pending !== undefined) env.frame.cancel(pending)
      pending = undefined
      root.remove()
    },
  }
}
