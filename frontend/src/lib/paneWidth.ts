// ペイン幅のリサイズ（最小幅 960px のウィンドウに収まるよう各ペインの幅を制限する）。

export const MIN_WINDOW_WIDTH = 960
export const SIDEBAR = { min: 200, max: 360, initial: 240 } as const
export const LIST = { min: 300, max: 560, initial: 380 } as const
/** 右ペイン（スレッド・作成）に最低限確保する幅 */
export const MIN_DETAIL = 320

export function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max)
}

/** サイドバー幅。右ペインの最小幅を割らないよう、一覧の現在幅も考慮する。 */
export function clampSidebar(width: number, listWidth: number, total: number): number {
  return clamp(
    width,
    SIDEBAR.min,
    Math.min(SIDEBAR.max, Math.max(SIDEBAR.min, total - listWidth - MIN_DETAIL)),
  )
}

/** 一覧幅。 */
export function clampList(width: number, sidebarWidth: number, total: number): number {
  return clamp(
    width,
    LIST.min,
    Math.min(LIST.max, Math.max(LIST.min, total - sidebarWidth - MIN_DETAIL)),
  )
}
