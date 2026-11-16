// メール本文の sandbox iframe の中のスクロールバー（#378）。iframe は別文書で、親のスタイルもトークン（CSS 変数）も受けない。
// そこで親が、同じスタイル（styles/overlayScroll.css）と色の変数を srcdoc に入れ（lib/sanitize の buildSrcdoc）、読み込み後に同じコントローラを取り付ける。
// iframe は color-scheme: light 固定で地が Canvas（白）なので、色は親のテーマ（ダーク）ではなく tokens.css の既定（Light）の --mute にする。
import overlayCss from '../../styles/overlayScroll.css?inline'
import scrollbarCss from '../../styles/scrollbar.css?inline'
import { attachOverlayScroll, browserEnv, type Env } from './controller'

/** CSS の色として注入してよい値（style を壊す文字を含まないもの）。 */
const SAFE_VALUE = /^[#\w(),.%\s/-]+$/

/** Android のタッチ UI は OS の重ねて出るバーを使うので、本文でも自前のバーは出さない。 */
function nativeOverlay(): boolean {
  return typeof document !== 'undefined' && document.documentElement.hasAttribute('data-touch-ui')
}

/** tokens.css の既定（Light）の値。@media の外の `:root` から読み、取れなければ親の現在の計算値にする。 */
function lightToken(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined
  let value = ''
  for (const sheet of Array.from(document.styleSheets)) {
    try {
      for (const rule of Array.from(sheet.cssRules)) {
        if (!('selectorText' in rule) || (rule as CSSStyleRule).selectorText !== ':root') continue
        value = (rule as CSSStyleRule).style.getPropertyValue(name).trim() || value
      }
    } catch {
      // 別オリジンのシートは cssRules に触れない。読めたものだけで決める。
    }
  }
  value ||= getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return SAFE_VALUE.test(value) ? value : undefined
}

/** srcdoc の `<style>` に入れる、本文のスクロールバーの CSS（色の変数＋ scrollbar.css ＋ overlayScroll.css。本文の中の入れ子のスクロール領域は scrollbar.css の既定、本文全体は重ねて出す）。 */
export function frameScrollbarCss(): string {
  const vars = `--mute:${lightToken('--mute') ?? 'GrayText'};--ink:${lightToken('--ink') ?? 'CanvasText'}`
  return `:root{${vars}}${scrollbarCss}${overlayCss}`
}

/**
 * srcdoc の `<html>` に付ける属性。`data-overlay-scroll` はネイティブのバーを隠す目印。タッチ UI（Android）では付けず、代わりに親と同じ
 * `data-touch-ui` を付ける（iframe は別文書で親の属性を受けないので、付けないと scrollbar.css の `:root:not([data-touch-ui])` が効き、OS の重ねて出るバーではなく太いバーを描く）。
 */
export function frameRootAttr(): string {
  return nativeOverlay() ? ' data-touch-ui' : ' data-overlay-scroll'
}

/** 読み込んだ iframe の文書に取り付ける。外すときの関数を返す。バーを自前で描かない（タッチ UI・属性なし）ときは undefined。 */
export function attachFrameOverlayScroll(
  doc: Document,
  env: Env = browserEnv,
): (() => void) | undefined {
  const root = doc.documentElement
  if (!root?.hasAttribute('data-overlay-scroll') || !doc.body) return undefined
  return attachOverlayScroll(root, {}, env).destroy
}
