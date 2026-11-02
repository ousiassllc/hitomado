// スレッド表示の純粋なロジック（メッセージの展開規則・宛先の整形）。
import type { ThreadDetail } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

interface Addr {
  name?: string
  email: string
}

type Msg = { id: string; isUnread: boolean }

/** 表示名があれば「名前 <アドレス>」、無ければアドレスのみ。 */
export function formatAddress(a: Addr): string {
  const name = a.name?.trim()
  return name ? `${name} <${a.email}>` : a.email
}

export function formatAddresses(list: Addr[]): string {
  return list.map(formatAddress).join(', ')
}

/** 折りたたみ表示用の短い差出人表記（表示名があれば名前、無ければアドレス）。 */
export function shortName(a: Addr): string {
  return a.name?.trim() || a.email
}

/** 最新と未読のメッセージを展開する（F-03）。 */
export function defaultExpanded(messages: Msg[]): Set<string> {
  const ids = new Set<string>()
  messages.forEach((m, i) => {
    if (m.isUnread || i === messages.length - 1) ids.add(m.id)
  })
  return ids
}

/**
 * スレッドの再取得に追従した展開状態を返す。別スレッドなら規則どおりに作り直し、同じスレッドの再取得では
 * ユーザーが開閉した状態を保ったまま、新しく増えたメッセージだけに規則を適用する
 * （自動既読化で isUnread が変わっても、開いていたカードが閉じない）。
 */
export function nextExpanded(
  prev: ThreadDetail | undefined,
  cur: ThreadDetail | undefined,
  expanded: ReadonlySet<string>,
): Set<string> {
  if (!cur) return new Set()
  if (!prev || prev.threadId !== cur.threadId || prev.account.id !== cur.account.id) {
    return defaultExpanded(cur.messages ?? [])
  }
  const known = new Set((prev.messages ?? []).map((m) => m.id))
  const msgs = cur.messages ?? []
  const next = new Set([...expanded].filter((id) => msgs.some((m) => m.id === id)))
  msgs.forEach((m, i) => {
    if (!known.has(m.id) && (m.isUnread || i === msgs.length - 1)) next.add(m.id)
  })
  return next
}
