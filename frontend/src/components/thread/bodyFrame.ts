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
  return el?.closest('a[href], area[href]')?.getAttribute('href') ?? undefined
}

export interface FrameBinding {
  /** 初回の高さ計測に成功したか。false の間は固定高＋内部スクロールのまま（計測できたら onHeight が呼ばれる） */
  measured: boolean
  release(): void
}

/**
 * iframe の文書にリンク捕捉と高さ計測を取り付ける。文書に触れない（SecurityError など）ときだけ undefined を
 * 返す。リンク捕捉は高さ計測の成否と独立で、常に取り付ける: クリックは常にナビゲーションを止め、許可スキーム
 * のみ onLink へ渡す。中クリック（auxclick の button===1）も同様に扱い、右クリックなど他のボタンは何もしない。
 */
export function bindFrameDocument(
  doc: Document | null | undefined,
  h: FrameHandlers,
): FrameBinding | undefined {
  try {
    if (!doc?.documentElement) return undefined

    const onClick = (ev: Event): void => {
      if (ev.type === 'auxclick' && (ev as MouseEvent).button !== 1) return
      const href = hrefAt(ev.target)
      if (href === undefined) return
      ev.preventDefault()
      if (isOpenableUrl(href)) h.onLink(href)
    }
    doc.addEventListener('click', onClick)
    doc.addEventListener('auxclick', onClick)

    const measure = (): boolean => {
      try {
        const px = measureContentHeight(doc)
        if (px === undefined) return false
        h.onHeight(px)
        return true
      } catch {
        return false
      }
    }
    // スクロールバーを出さず、親側の iframe 高さで全体を見せる。
    const measured = measure()
    if (measured) doc.documentElement.style.overflow = 'hidden'

    let observer: ResizeObserver | undefined
    try {
      if (typeof ResizeObserver === 'function') {
        observer = new ResizeObserver(measure)
        observer.observe(doc.documentElement)
        if (doc.body) observer.observe(doc.body)
      }
    } catch {
      observer = undefined
    }
    // 画像の遅延読み込みなど、ResizeObserver が拾えないケースの保険（load はバブルしないので capture）。
    doc.addEventListener('load', measure, true)

    return {
      measured,
      release: () => {
        doc.removeEventListener('click', onClick)
        doc.removeEventListener('auxclick', onClick)
        doc.removeEventListener('load', measure, true)
        observer?.disconnect()
      },
    }
  } catch {
    return undefined
  }
}
