// 画面幅によるレイアウトの切り替え（ブレイクポイントはここだけ）。
//   wide   : 3 ペイン（サイドバー・一覧・スレッド）。Windows 版は常にこれ（最小幅 960px のウィンドウ）
//   medium : 2 ペイン（一覧・スレッド）。サイドバーはドロワー。Fold を開いたときなどの中くらいの幅
//   stack  : 1 画面ずつ（一覧 → スレッド → 返信）。サイドバーはドロワー
import { MIN_WINDOW_WIDTH } from './paneWidth'

export type LayoutMode = 'wide' | 'medium' | 'stack'

/** この幅以上は 3 ペイン（デスクトップの最小ウィンドウ幅と同じ）。 */
export const BREAKPOINT_WIDE = MIN_WINDOW_WIDTH
/** この幅以上は 2 ペイン、未満は 1 画面ずつ。 */
export const BREAKPOINT_MEDIUM = 600

/** matchMedia に渡すクエリ。各幅以上で true。 */
export const WIDE_QUERY = `(min-width: ${BREAKPOINT_WIDE}px)`
export const MEDIUM_QUERY = `(min-width: ${BREAKPOINT_MEDIUM}px)`

/**
 * レイアウトを決める。タッチ UI（Android）でないときは幅に関わらず常に 3 ペイン（デスクトップの表示を変えない）。
 * wide・medium はそれぞれ WIDE_QUERY・MEDIUM_QUERY の結果。
 */
export function layoutModeFor(touchUi: boolean, wide: boolean, medium: boolean): LayoutMode {
  if (!touchUi || wide) return 'wide'
  return medium ? 'medium' : 'stack'
}
