// アカウント色パレット。キーは Go 側（internal/domain/palette）と同じ順序・同じ値。
// 実際の色値は styles/tokens.css の --account-<key> / --account-<key>-text で定義する。

export const ACCOUNT_COLOR_KEYS = [
  'red',
  'orange',
  'amber',
  'green',
  'teal',
  'cyan',
  'blue',
  'indigo',
  'purple',
  'pink',
] as const

export type AccountColorKey = (typeof ACCOUNT_COLOR_KEYS)[number]

export function isAccountColorKey(key: string): key is AccountColorKey {
  return (ACCOUNT_COLOR_KEYS as readonly string[]).includes(key)
}

/** 帯・チップ背景の CSS 値。 */
export function accountColorVar(key: string): string {
  return `var(--account-${key})`
}

/** `accountColorVar` の上に載せる文字色の CSS 値。 */
export function accountTextVar(key: string): string {
  return `var(--account-${key}-text)`
}
