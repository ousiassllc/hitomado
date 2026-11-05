// アーカイブ・ゴミ箱・既読/未読・ラベルの楽観的更新。リストから即時反映し、失敗時は元へ戻してエラーアラート。
// アーカイブ・ゴミ箱は成功直後にスナックバー（「元に戻す」）を出し、戻すと Gmail 側も元の状態に戻す（ガイド O3）。
import type { Ref } from 'vue'
import type {
  ThreadRef,
  ThreadSummary,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import type { AppError } from '../lib/api'
import { list } from '../lib/api'
import { failedRefsOf } from '../lib/errorEffects'
import { keyOf, removeKeys, restoreRemoved } from '../lib/inboxItems'
import { common } from '../lib/messages'
import type { useUiStore } from './ui'

type Op = 'archive' | 'trash' | 'read' | 'unread'

export interface MutationDeps {
  items: Ref<ThreadSummary[]>
  /** 未読のみ表示中の受信箱か（既読にした行が一覧から外れる） */
  hidesRead: () => boolean
  /** 検索結果の表示中か（元に戻したスレッドを、受信箱の条件に関わらず一覧へ戻す） */
  searching: () => boolean
  /** サーバー反映待ちで楽観的に除去した行。先頭ページの再取得で復活させない。 */
  pendingRemoval: Set<string>
  api: <T>(p: PromiseLike<T>) => Promise<T>
  ui: ReturnType<typeof useUiStore>
  /** 開いているスレッドが除去対象のとき、'next' は次の行を開く。戻り値は失敗時に元のスレッドを開き直す関数。 */
  afterRemoval: (
    keys: ReadonlySet<string>,
    kept: ThreadSummary[],
    firstIndex: number,
    opened: 'next' | 'close' | 'keep',
  ) => (failed?: Set<string>) => void
  /** 操作後に先頭ページと未読件数を再取得する */
  reload: () => void
}

const OP_FAILED: Record<Op, string> = {
  archive: common.op.archiveFailed,
  trash: common.op.trashFailed,
  read: common.op.markReadFailed,
  unread: common.op.markReadFailed,
}

export function useInboxMutations(d: MutationDeps) {
  function callOp(op: Op, refs: ThreadRef[]): Promise<ThreadSummary[] | null> {
    switch (op) {
      case 'archive':
        return d.api(InboxService.Archive(refs))
      case 'trash':
        return d.api(InboxService.Trash(refs))
      default:
        return d.api(InboxService.MarkRead(refs, op === 'read'))
    }
  }

  /** 楽観的に反映し、元に戻すための関数（失敗した ref の集合があればそれだけ）を返す。 */
  function applyOptimistic(op: Op, keys: Set<string>): (failed?: Set<string>) => void {
    const removes = op === 'archive' || op === 'trash' || (op === 'read' && d.hidesRead())
    if (!removes) {
      const prev = new Map<string, boolean>()
      d.items.value = d.items.value.map((t) => {
        if (!keys.has(keyOf(t))) return t
        prev.set(keyOf(t), t.isUnread)
        return { ...t, isUnread: op === 'unread' }
      })
      return (failed) => {
        d.items.value = d.items.value.map((t) => {
          const k = keyOf(t)
          return prev.has(k) && (!failed || failed.has(k))
            ? { ...t, isUnread: prev.get(k) === true }
            : t
        })
      }
    }
    const r = removeKeys(d.items.value, keys)
    d.items.value = r.items
    for (const k of keys) d.pendingRemoval.add(k)
    // アーカイブ・ゴミ箱は次の行を開く。既読化で行が消えるだけ（未読のみ表示中）なら、開いているスレッドは変えない
    const restoreSelection = d.afterRemoval(
      keys,
      r.items,
      r.removed[0]?.index ?? 0,
      op === 'read' ? 'keep' : 'next',
    )
    return (failed) => {
      const back = failed ? r.removed.filter((x) => failed.has(keyOf(x.item))) : r.removed
      d.items.value = restoreRemoved(d.items.value, back)
      restoreSelection(failed)
    }
  }

  async function mutate(op: Op, refs: ThreadRef[]): Promise<boolean> {
    if (refs.length === 0) return true
    const keys = new Set(refs.map(keyOf))
    const wasInInbox = new Map(
      d.items.value.filter((t) => keys.has(keyOf(t))).map((t) => [keyOf(t), t.inInbox]),
    )
    const undo = applyOptimistic(op, keys)
    try {
      const updated = await callOp(op, refs)
      if (op === 'read' || op === 'unread') {
        const byKey = new Map(list(updated).map((t) => [keyOf(t), t]))
        d.items.value = d.items.value.map((t) => byKey.get(keyOf(t)) ?? t)
      } else if (op === 'archive' || op === 'trash') {
        announceRemoval(op, refs, wasInInbox)
      }
      return true
    } catch (e) {
      const err = e as AppError
      const failedRefs = failedRefsOf(err)
      const failed = failedRefs.length > 0 ? new Set(failedRefs.map(keyOf)) : undefined
      if (err.code === 'NOT_FOUND') {
        // NOT_FOUND は失敗が全件 NOT_FOUND のときだけ。failedRefs に無い ref は Go 側で成功済みなので何も復元しない。
        // 存在しなかったスレッド（failedRefs、無ければ対象全件）は一覧から外す（読み取りだけの markRead 等で残っていても）。
        const gone = failed ?? keys
        undo(new Set())
        d.items.value = d.items.value.filter((t) => !gone.has(keyOf(t)))
        d.ui.pushAlert(common.op.threadGone)
      } else {
        undo(failed)
        const retryRefs = failed ? failedRefs : refs
        d.ui.pushAlert(`${OP_FAILED[op]}${err.message}`, () => void mutate(op, retryRefs))
      }
      return false
    } finally {
      for (const k of keys) d.pendingRemoval.delete(k)
      d.reload()
    }
  }

  /** 元に戻したスレッドを、今の検索か受信箱の表示条件（検索中か・未読のみか）に合うものだけ一覧へ差し戻す（先頭ページの再取得より先に見せる。アカウントの絞り込みは直後の再取得で正される）。 */
  function reinsert(restored: ThreadSummary[]): void {
    const fits = (t: ThreadSummary) =>
      d.searching() || (t.inInbox && (!d.hidesRead() || t.isUnread))
    const back = restored.filter(fits).map((item) => ({ item, index: 0 }))
    d.items.value = restoreRemoved(d.items.value, back)
  }

  /** アーカイブ・ゴミ箱の取り消し。`backToInbox` はゴミ箱を戻すとき INBOX も付け直すか。失敗はエラーアラート（再試行つき）。 */
  async function undoRemoval(
    op: 'archive' | 'trash',
    refs: ThreadRef[],
    backToInbox: boolean,
  ): Promise<void> {
    try {
      const restored =
        op === 'archive'
          ? await d.api(InboxService.Unarchive(refs))
          : await d.api(InboxService.Untrash(refs, backToInbox))
      reinsert(list(restored))
    } catch (e) {
      const err = e as AppError
      if (err.code === 'NOT_FOUND') {
        d.ui.pushAlert(common.op.threadGone)
      } else {
        const failed = failedRefsOf(err)
        const retryRefs = failed.length > 0 ? failed : refs
        d.ui.pushAlert(
          `${common.op.undoFailed}${err.message}`,
          () => void undoRemoval(op, retryRefs, backToInbox),
        )
      }
    } finally {
      d.reload()
    }
  }

  /** 成功したアーカイブ・ゴミ箱をスナックバーで知らせる。`wasInInbox` は一覧にあった行の受信箱判定（一覧に無ければ受信箱にあったとみなす）。 */
  function announceRemoval(
    op: 'archive' | 'trash',
    refs: ThreadRef[],
    wasInInbox: Map<string, boolean>,
  ): void {
    const message =
      op === 'archive' ? common.op.archived(refs.length) : common.op.trashed(refs.length)
    const inbox = refs.filter((r) => wasInInbox.get(keyOf(r)) !== false)
    const others = refs.filter((r) => wasInInbox.get(keyOf(r)) === false)
    // アーカイブは INBOX を外すだけなので、受信箱に無かったスレッドは何も変わっておらず、取り消す対象が無い（「元に戻す」は出さない）
    if (op === 'archive' && inbox.length === 0) {
      d.ui.pushToast('success', message)
      return
    }
    const undo = async () => {
      if (inbox.length > 0) await undoRemoval(op, inbox, true)
      if (op === 'trash' && others.length > 0) await undoRemoval(op, others, false)
    }
    d.ui.pushSnackbar(message, { label: common.op.undo, run: () => void undo() })
  }

  /** ラベルの付与／除去（Could）。結果の ThreadSummary で一覧の行を差し替える。 */
  async function modifyLabel(refs: ThreadRef[], labelId: string, add: boolean): Promise<boolean> {
    let ok = true
    for (const ref of refs) {
      try {
        const updated = await d.api(
          InboxService.ModifyLabels(ref, add ? [labelId] : [], add ? [] : [labelId]),
        )
        d.items.value = d.items.value.map((t) => (keyOf(t) === keyOf(updated) ? updated : t))
      } catch (e) {
        ok = false
        d.ui.pushAlert(`${common.op.labelFailed}${(e as AppError).message}`)
        break
      }
    }
    return ok
  }

  return {
    archive: (refs: ThreadRef[]) => mutate('archive', refs),
    trash: (refs: ThreadRef[]) => mutate('trash', refs),
    markRead: (refs: ThreadRef[], read: boolean) => mutate(read ? 'read' : 'unread', refs),
    modifyLabel,
  }
}
