// 受信箱リストの純粋な操作（楽観的更新・復元・先頭ページの差し替え・範囲選択）。
import type {
  ThreadRef,
  ThreadSummary,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

export function threadKey(accountId: string, threadId: string): string {
  return `${accountId}:${threadId}`
}

export function keyOf(t: ThreadRef | ThreadSummary): string {
  return threadKey(t.accountId, t.threadId)
}

/** 一覧の表示条件（モード・検索語・タブ・未読のみ・アカウント）を表す文字列。条件が変わったかの比較に使う。 */
export function viewKeyOf(
  mode: string,
  searchQuery: string,
  tab: string,
  filter: { unreadOnly: boolean; accountIds: string[] },
): string {
  return JSON.stringify([mode, searchQuery, tab, filter.unreadOnly, filter.accountIds])
}

export function refOf(t: ThreadRef | ThreadSummary): ThreadRef {
  return { accountId: t.accountId, threadId: t.threadId }
}

/** キー文字列から ThreadRef を復元する。アカウント ID・スレッド ID は ':' を含まない前提（ULID／Gmail ID）。 */
export function refFromKey(key: string): ThreadRef | undefined {
  const i = key.indexOf(':')
  if (i <= 0 || i === key.length - 1) return undefined
  return { accountId: key.slice(0, i), threadId: key.slice(i + 1) }
}

export interface Removed {
  item: ThreadSummary
  index: number
}

/** keys に含まれる行を取り除く。復元用に元の位置も返す。 */
export function removeKeys(
  items: ThreadSummary[],
  keys: ReadonlySet<string>,
): { items: ThreadSummary[]; removed: Removed[] } {
  const kept: ThreadSummary[] = []
  const removed: Removed[] = []
  items.forEach((item, index) => {
    if (keys.has(keyOf(item))) removed.push({ item, index })
    else kept.push(item)
  })
  return { items: kept, removed }
}

/**
 * 取り除いた行を戻す。リストは lastMessageAt 降順なので、その順序を保つ位置に挿入する
 * （復元までの間に先頭ページが差し替わっていても整合する）。すでに存在する行は無視する。
 */
export function restoreRemoved(current: ThreadSummary[], removed: Removed[]): ThreadSummary[] {
  const out = [...current]
  const present = new Set(out.map(keyOf))
  for (const { item } of [...removed].sort((a, b) => a.index - b.index)) {
    const k = keyOf(item)
    if (present.has(k)) continue
    let pos = out.findIndex((o) => o.lastMessageAt < item.lastMessageAt)
    if (pos < 0) pos = out.length
    out.splice(pos, 0, item)
    present.add(k)
  }
  return out
}

/**
 * 先頭ページ `head` で現在のリストを更新する。続き（`hasMore`）がある場合は、先頭ページより古い既読込分を残す。
 * `exclude` は処理中（楽観的に除去済み）の行で、サーバー反映前の古い状態で復活させない。
 */
export function mergeHead(
  old: ThreadSummary[],
  head: ThreadSummary[],
  hasMore: boolean,
  exclude: ReadonlySet<string> = new Set(),
): { items: ThreadSummary[]; keptTail: boolean } {
  const fresh = head.filter((t) => !exclude.has(keyOf(t)))
  if (!hasMore || head.length === 0) return { items: fresh, keptTail: false }
  const oldest = head[head.length - 1].lastMessageAt
  const headKeys = new Set(head.map(keyOf))
  const tail = old.filter((t) => t.lastMessageAt < oldest && !headKeys.has(keyOf(t)))
  return { items: [...fresh, ...tail], keptTail: tail.length > 0 }
}

/** items 上の 2 キー間（両端を含む）のキー。どちらかが無ければ to のみ。 */
export function rangeKeys(items: ThreadSummary[], from: string | undefined, to: string): string[] {
  const a = from === undefined ? -1 : items.findIndex((t) => keyOf(t) === from)
  const b = items.findIndex((t) => keyOf(t) === to)
  if (b < 0) return []
  if (a < 0) return [to]
  const [lo, hi] = a <= b ? [a, b] : [b, a]
  return items.slice(lo, hi + 1).map(keyOf)
}

/** 行を除去した後のカーソル位置: 除去した最初の位置にあった次の行、無ければ最後の行。 */
export function cursorAfterRemoval(
  kept: ThreadSummary[],
  removedFirstIndex: number,
): string | undefined {
  if (kept.length === 0) return undefined
  return keyOf(kept[Math.min(removedFirstIndex, kept.length - 1)])
}
