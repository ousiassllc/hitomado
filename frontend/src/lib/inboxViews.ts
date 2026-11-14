import type {
  ThreadPage,
  ThreadSummary,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { list } from './api'
import { common } from './messages'

/**
 * 「見る場所」（サイドバー・下タブの 受信箱／下書き／送信済み／アーカイブ）。アカウントの絞り込みとは別の軸で、
 * 組み合わせて効く（例: 仕事アカウントの送信済みだけ）。受信箱は同期済みの一覧（ListInbox）、送信済み・アーカイブは Gmail の検索（Search）で引く。
 * 下書きは、下書きの一覧ページ（#366。PR #367）が入るまで「準備中」（場所と並びだけ置く）。Gmail の `in:drafts` の検索は、
 * 新規作成だけの下書き（下書きのメッセージ 1 通だけのスレッド）を返さず、返信の下書きでも直前の受信メールを行に出すので、代わりには使わない。
 */
export const INBOX_VIEWS = ['inbox', 'drafts', 'sent', 'archive'] as const

export type InboxView = (typeof INBOX_VIEWS)[number]

/**
 * 送信済み・アーカイブの Gmail 検索クエリ。Gmail は「スレッドのどれか 1 通が条件に合えば」そのスレッドを返し、否定語もメッセージごとに判定される。
 * そのためアーカイブは `-in:inbox` で引いたあと、受信箱にもあるスレッド（`inInbox`）を取り込み時に落とす（keepsThread）。
 * ゴミ箱・迷惑メールは検索の既定（includeSpamTrash=false）で除かれる。
 */
const VIEW_QUERY = { sent: 'in:sent', archive: '-in:inbox' } as const

/** 「未読のみ」が効く場所。送信済み・下書きは自分が送った／書いたメールで未読にならず、掛けると必ず空になるので効かせない（スイッチも出さない）。 */
export function viewShowsUnreadOnly(view: InboxView): boolean {
  return view === 'inbox' || view === 'archive'
}

/** 送信済み・アーカイブの検索クエリ（アーカイブで未読のみのときは `is:unread` を足す）。受信箱は ListInbox、下書きは準備中なので undefined。 */
export function viewSearchQuery(view: InboxView, unreadOnly: boolean): string | undefined {
  if (view !== 'sent' && view !== 'archive') return undefined
  return unreadOnly && viewShowsUnreadOnly(view)
    ? `${VIEW_QUERY[view]} is:unread`
    : VIEW_QUERY[view]
}

/** 検索で引いた行をその場所の一覧に入れるか。アーカイブは受信箱にもあるスレッドを入れない。 */
function keepsThread(view: InboxView, t: ThreadSummary): boolean {
  return view !== 'archive' || !t.inInbox
}

/**
 * 受信箱以外の場所の 1 ページ（`search` は Gmail の検索）。受信箱は undefined（ListInbox で引く）。
 * 下書きは、下書きの一覧ページ（#366）が入るまで準備中なので何も引かない。
 */
export async function fetchViewPage(
  view: InboxView,
  unreadOnly: boolean,
  search: (q: string) => Promise<ThreadPage>,
): Promise<ThreadPage | undefined> {
  if (view === 'drafts') return { items: [], failures: [] }
  const q = viewSearchQuery(view, unreadOnly)
  if (q === undefined) return undefined
  const page = await search(q)
  return { ...page, items: list(page.items).filter((t) => keepsThread(view, t)) }
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
  if (mode !== 'inbox') return false
  return (unreadOnly && viewShowsUnreadOnly(view)) || (view === 'inbox' && tab === 'new')
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
