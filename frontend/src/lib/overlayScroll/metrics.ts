// スクロールバーのつまみの寸法（#378）。DOM を知らない純粋な計算。

/** つまみの端からの内側の余白（px）。つまみの長さの両端・脇の 2px。 */
export const INSET = 2
/** つまみの長さの最小値（px）。 */
export const MIN_THUMB = 32
/** 縦横の両方が出るとき、隅に空ける長さ（つまみの当たり判定の幅）。 */
export const CORNER = 12
/** はみ出しとみなす最小の差（px）。サブピクセルの丸め誤差でバーを出さない。 */
const EPSILON = 1

export interface Axis {
  /** scrollTop／scrollLeft */
  scroll: number
  /** scrollHeight／scrollWidth */
  size: number
  /** clientHeight／clientWidth */
  client: number
}

export interface Thumb {
  /** つまみの長さ（px） */
  length: number
  /** エリアの端からつまみの先頭まで（px） */
  offset: number
  /** つまみが動ける長さ（px）。ドラッグの換算に使う */
  travel: number
}

export function overflows(a: Axis): boolean {
  return a.size - a.client > EPSILON
}

/** はみ出していなければ null。`reserve` は反対側の軸のバーのために空ける長さ。 */
export function thumbFor(a: Axis, reserve: number): Thumb | null {
  if (!overflows(a)) return null
  const track = a.client - INSET * 2 - reserve
  if (track <= 0) return null
  const length = Math.min(track, Math.max(MIN_THUMB, (track * a.client) / a.size))
  const ratio = Math.min(1, Math.max(0, a.scroll / (a.size - a.client)))
  const travel = track - length
  return { length, offset: INSET + travel * ratio, travel }
}

/** つまみを `delta`px 動かしたときの、スクロール位置の変化量。 */
export function scrollDeltaFor(a: Axis, thumb: Thumb, delta: number): number {
  return thumb.travel > 0 ? (delta * (a.size - a.client)) / thumb.travel : 0
}

/** 続きがある側。いちばん端まで来たらその側は false。 */
export function hasMore(a: Axis): { before: boolean; after: boolean } {
  return {
    before: overflows(a) && a.scroll > EPSILON,
    after: overflows(a) && a.scroll + a.client < a.size - EPSILON,
  }
}
