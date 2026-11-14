import { common } from './messages'

/**
 * 「見る場所」（サイドバー・下タブの 受信箱／下書き／送信済み／アーカイブ）。アカウントの絞り込みとは別の軸で、
 * 組み合わせて効く（例: 仕事アカウントの下書きだけ）。受信箱は同期済みの一覧（ListInbox）、ほかは Gmail の検索（Search）で引く。
 * 下書きの一覧ページ（#366。PR #367）が入ったら、drafts の中身はそちらに差し替える（場所と並びは変えない）。
 */
export const INBOX_VIEWS = ['inbox', 'drafts', 'sent', 'archive'] as const

export type InboxView = (typeof INBOX_VIEWS)[number]

/** 受信箱以外の場所の Gmail 検索クエリ。アーカイブは「受信箱・送信済み・下書きのどれでもない」（ゴミ箱・迷惑メールは検索の既定で除かれる）。 */
const VIEW_QUERY = {
  drafts: 'in:drafts',
  sent: 'in:sent',
  archive: '-in:inbox -in:sent -in:drafts',
} as const

/** 受信箱以外の場所の検索クエリ（未読のみのときは `is:unread` を足す）。受信箱は ListInbox で引くので undefined。 */
export function viewSearchQuery(view: InboxView, unreadOnly: boolean): string | undefined {
  if (view === 'inbox') return undefined
  return unreadOnly ? `${VIEW_QUERY[view]} is:unread` : VIEW_QUERY[view]
}

/** 読み込みの失敗の文言（検索・受信箱・ほかの場所で出し分ける）。 */
export function loadFailedMessageOf(mode: 'inbox' | 'search', view: InboxView): string {
  if (mode === 'search') return common.op.searchFailed
  return view === 'inbox' ? common.op.loadFailed : common.op.viewLoadFailed
}

/** 既読にした行が一覧から外れるか（未読のみ、または受信箱の「新着」タブ。検索結果は外れない）。 */
export function hidesReadOf(
  mode: 'inbox' | 'search',
  view: InboxView,
  tab: string,
  unreadOnly: boolean,
): boolean {
  return mode === 'inbox' && (unreadOnly || (view === 'inbox' && tab === 'new'))
}

/** 受信箱の未読数。アカウントを絞り込んでいるときは、選んだアカウントの分だけ（未選択は全アカウント）。 */
export function unreadTotalOf(
  counts: Record<string, number>,
  accountIds: readonly string[],
): number {
  return Object.entries(counts).reduce(
    (n, [id, c]) => (accountIds.length === 0 || accountIds.includes(id) ? n + c : n),
    0,
  )
}
