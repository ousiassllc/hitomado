// 本文 iframe（sandbox="allow-same-origin" のみ・スクリプト不可）に対するホスト側の処理。
// 親が contentDocument に触れて (a) 内容の高さを測り (b) リンクのクリックを捕捉する（docs §3.8）。
// WebView2 で動かない場合に備え、失敗は握りつぶさず undefined を返して呼び出し側が固定高＋内部スクロール
// （docs §3.8 の代替案①）へフォールバックできるようにする。
import { isOpenableUrl } from '../../lib/sanitize'

/** 高さ計測が使えないときの固定高（px）。iframe 内部でスクロールする。 */
export const FALLBACK_HEIGHT = 480
/** 計測した高さの下限（px）。 */
const MIN_HEIGHT = 24

export interface FrameHandlers {
  /** 計測した内容の高さ（px）。内容の変化（画像の読み込みなど）で繰り返し呼ばれる */
  onHeight(px: number): void
  /** 開いてよいリンク（http／https／mailto）がクリックされた */
  onLink(url: string): void
}

/** 内容の高さ。計測できない（値が不正・0）ときは undefined。 */
export function measureContentHeight(doc: Document): number | undefined {
  const body = doc.body
  if (!body) return undefined
  // documentElement.scrollHeight はビューポート高以上になり縮められないので、body の高さで測る。
  const h = Math.max(body.scrollHeight, body.offsetHeight)
  return Number.isFinite(h) && h > 0 ? Math.max(Math.ceil(h), MIN_HEIGHT) : undefined
}

/** クリックされた位置から最も近い `<a href>` の href を返す。iframe の要素は親と別 realm なので instanceof は使わない。 */
function hrefAt(target: EventTarget | null): string | undefined {
  const node = target as Node | null
  const el = node?.nodeType === 1 ? (node as Element) : (node?.parentElement ?? null)
  return el?.closest('a[href]')?.getAttribute('href') ?? undefined
}

/**
 * iframe の文書にリンク捕捉と高さ計測を取り付ける。成功したら解除関数を返し、失敗（文書に触れない・
 * 高さが測れない）なら undefined を返す。リンクは常にナビゲーションを止め、許可スキームのみ onLink へ渡す。
 */
export function bindFrameDocument(
  doc: Document | null | undefined,
  h: FrameHandlers,
): (() => void) | undefined {
  try {
    if (!doc) return undefined
    const first = measureContentHeight(doc)
    if (first === undefined) return undefined

    const onClick = (ev: Event): void => {
      const href = hrefAt(ev.target)
      if (href === undefined) return
      ev.preventDefault()
      if (isOpenableUrl(href)) h.onLink(href)
    }
    doc.addEventListener('click', onClick)
    doc.addEventListener('auxclick', onClick)
    // スクロールバーを出さず、親側の iframe 高さで全体を見せる。
    doc.documentElement.style.overflow = 'hidden'
    h.onHeight(first)

    const measure = (): void => {
      const px = measureContentHeight(doc)
      if (px !== undefined) h.onHeight(px)
    }
    let observer: ResizeObserver | undefined
    if (typeof ResizeObserver === 'function') {
      observer = new ResizeObserver(measure)
      observer.observe(doc.documentElement)
      if (doc.body) observer.observe(doc.body)
    }
    // 画像の遅延読み込みなど、ResizeObserver が拾えないケースの保険（load はバブルしないので capture）。
    doc.addEventListener('load', measure, true)

    return () => {
      doc.removeEventListener('click', onClick)
      doc.removeEventListener('auxclick', onClick)
      doc.removeEventListener('load', measure, true)
      observer?.disconnect()
    }
  } catch {
    return undefined
  }
}
