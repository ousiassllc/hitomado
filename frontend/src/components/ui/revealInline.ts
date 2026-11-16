// 横にスクロールする行（設定ダイアログのタブ行など）で、選んでいる項目が見える位置に来るようにスクロールする（#383）。
// 変形（ダイアログの開くアニメーション）の影響を受けないよう、座標は offsetLeft／offsetWidth（scroller が項目の offsetParent）で見る。

/** 項目を、スクロールの量（scrollLeft）が最小で済む向きで見える範囲に入れる。すでに見えていれば動かさない。 */
export function revealInline(
  scroller: Pick<HTMLElement, 'scrollLeft' | 'clientWidth'>,
  item: Pick<HTMLElement, 'offsetLeft' | 'offsetWidth'>,
): void {
  const left = item.offsetLeft
  const right = left + item.offsetWidth
  if (left < scroller.scrollLeft) scroller.scrollLeft = left
  else if (right > scroller.scrollLeft + scroller.clientWidth)
    scroller.scrollLeft = right - scroller.clientWidth
}
