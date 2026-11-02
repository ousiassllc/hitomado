import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import type {
  ThreadDetail,
  ThreadRef,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as ThreadService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/threadservice'
import { type AppError, call, list, parseBindingError } from '../lib/api'
import { applyErrorEffects } from '../lib/errorEffects'
import {
  EVENTS,
  type NotifyOpenThreadPayload,
  onEvent,
  type SyncUpdatedPayload,
} from '../lib/events'
import { threadKey } from '../lib/inboxItems'
import { thread as msg } from '../lib/messages'
import { useAccountsStore } from './accounts'
import { useInboxStore } from './inbox'
import { useThreadExtras } from './threadExtras'
import { useUiStore } from './ui'

/** null（Go の nil スライス）を空配列にして、画面側で `?? []` を不要にする。 */
function normalize(d: ThreadDetail): ThreadDetail {
  return {
    ...d,
    labels: list(d.labels),
    messages: list(d.messages).map((m) => ({
      ...m,
      to: list(m.to),
      cc: list(m.cc),
      attachments: list(m.attachments),
    })),
  }
}

export const useThreadStore = defineStore('thread', () => {
  const ui = useUiStore()
  const accounts = useAccountsStore()
  const inbox = useInboxStore()

  const current = ref<ThreadDetail>()
  const currentRef = ref<ThreadRef>()
  const loading = ref(false)
  const error = ref<AppError>()
  /** 表示中スレッドに未読があるか（自動既読化・操作の結果を即反映するため、messages とは別に持つ） */
  const unread = ref(false)

  let seq = 0
  /** 既読化・未読化の呼び出し中の件数（スレッドキー単位。別スレッドの表示をブロックしない） */
  const marking = new Map<string, number>()
  /**
   * 表示中スレッドで自動既読化を済ませた（または既読だった）メッセージ ID。再取得で増えた未読だけを自動既読に
   * する（手動で「未読にする」を戻さない）。未読メッセージは既読化の確定後にだけ追加する。
   */
  const seenIds = new Set<string>()
  /** 自動既読化に失敗したメッセージ ID。同じ ID の自動再試行を抑止する（手動の既読／未読操作でクリア）。 */
  const failedIds = new Set<string>()
  let started = false
  let stops: (() => void)[] = []

  const currentKey = computed(() =>
    currentRef.value ? threadKey(currentRef.value.accountId, currentRef.value.threadId) : undefined,
  )

  function noteError(e: unknown): AppError {
    const err = parseBindingError(e)
    applyErrorEffects(err, { setOnline: ui.setOnline, markNeedsReauth: accounts.markNeedsReauth })
    return err
  }

  async function api<T>(p: PromiseLike<T>): Promise<T> {
    try {
      const v = await call(p)
      if (!ui.online) ui.setOnline(true)
      return v
    } catch (e) {
      throw noteError(e)
    }
  }

  function reset(): void {
    seq++
    current.value = undefined
    currentRef.value = undefined
    loading.value = false
    error.value = undefined
    unread.value = false
    seenIds.clear()
    failedIds.clear()
  }

  function beginMark(key: string): void {
    marking.set(key, (marking.get(key) ?? 0) + 1)
  }

  function endMark(key: string): void {
    const n = (marking.get(key) ?? 1) - 1
    if (n > 0) marking.set(key, n)
    else marking.delete(key)
  }

  /** 表示中スレッドで、まだ既読化していない未読メッセージの ID。 */
  function pendingIds(): string[] {
    return list(current.value?.messages)
      .filter((m) => m.isUnread && !seenIds.has(m.id))
      .map((m) => m.id)
  }

  /** 未処理の未読に、まだ失敗していないもの（＝自動既読化を試みてよいもの）があるか。 */
  function hasRetriable(ids: string[]): boolean {
    return ids.some((id) => !failedIds.has(id))
  }

  /**
   * 表示後に未読があれば既読にする（inboxStore 経由。失敗時のトーストと復元は inboxStore が行う）。
   * 終了時（成功・失敗とも）に表示中の messages から未処理の未読を再評価して unread を正しく保つ。
   * 実行中に届いた新着の未読は、確定後にあらためて既読化する。失敗した ID は再試行しない。
   */
  async function autoMarkRead(ref: ThreadRef, ids: string[]): Promise<void> {
    const key = threadKey(ref.accountId, ref.threadId)
    beginMark(key)
    let ok = false
    try {
      ok = await inbox.markRead([ref], true)
    } finally {
      endMark(key)
    }
    // 表示が別スレッドに移っていたら（seenIds・failedIds は表示中スレッドのもの）何も反映しない
    if (currentKey.value !== key) return
    for (const id of ids) {
      if (ok) {
        seenIds.add(id)
        failedIds.delete(id)
      } else {
        failedIds.add(id)
      }
    }
    const pending = pendingIds()
    unread.value = pending.length > 0
    if (hasRetriable(pending) && !marking.has(key)) void autoMarkRead(ref, pending)
  }

  async function load(ref: ThreadRef): Promise<void> {
    const my = ++seq
    const key = threadKey(ref.accountId, ref.threadId)
    if (!current.value) loading.value = true
    try {
      const d = normalize(await api(ThreadService.GetThread(ref)))
      if (my !== seq) return
      const msgs = list(d.messages)
      for (const m of msgs) {
        if (!m.isUnread) {
          seenIds.add(m.id)
          failedIds.delete(m.id)
        }
      }
      current.value = d
      error.value = undefined
      const pending = pendingIds()
      const busy = marking.has(key)
      if (!busy) unread.value = msgs.some((m) => m.isUnread)
      if (!busy && hasRetriable(pending)) void autoMarkRead(ref, pending)
    } catch (e) {
      if (my !== seq) return
      const err = e as AppError
      if (err.code === 'NOT_FOUND') {
        ui.pushToast('info', msg.notFound)
        reset()
        inbox.removeThread({ accountId: ref.accountId, threadId: ref.threadId })
        return
      }
      // 再取得の失敗では表示中の内容を残す（オフライン時に取得済みの本文を見られるように）
      if (!current.value) error.value = err
    } finally {
      if (my === seq) loading.value = false
    }
  }

  /** スレッドを開く。同じスレッドを表示中・読み込み中なら何もしない。 */
  async function open(ref: ThreadRef): Promise<void> {
    const key = threadKey(ref.accountId, ref.threadId)
    if (currentKey.value === key && (current.value || loading.value)) return
    reset()
    currentRef.value = { accountId: ref.accountId, threadId: ref.threadId }
    await load(currentRef.value)
  }

  function close(): void {
    reset()
  }

  async function reload(): Promise<void> {
    if (currentRef.value) await load(currentRef.value)
  }

  const extras = useThreadExtras({ currentRef, current, ui, inbox, api, reload })

  async function archive(): Promise<boolean> {
    return currentRef.value ? inbox.archive([currentRef.value]) : false
  }

  async function trash(): Promise<boolean> {
    return currentRef.value ? inbox.trash([currentRef.value]) : false
  }

  /** 既読⇔未読を切り替える。 */
  async function toggleRead(): Promise<boolean> {
    const ref = currentRef.value
    if (!ref) return false
    const key = threadKey(ref.accountId, ref.threadId)
    const toRead = unread.value
    failedIds.clear() // 手動操作で自動再試行の抑止を解除する
    beginMark(key)
    try {
      const ok = await inbox.markRead([ref], toRead)
      if (ok && currentKey.value === key) unread.value = !toRead
      return ok
    } finally {
      endMark(key)
    }
  }

  function onSyncUpdated(p: SyncUpdatedPayload): void {
    const ref = currentRef.value
    if (ref && p.accountId === ref.accountId && p.changedThreadIds.includes(ref.threadId))
      void reload()
  }

  async function onNotifyOpen(p: NotifyOpenThreadPayload): Promise<void> {
    const ref = { accountId: p.accountId, threadId: p.threadId }
    inbox.select(ref)
    await open(ref)
  }

  /** イベント購読と、一覧の選択に追従する監視を開始する（冪等）。 */
  function start(): void {
    if (started) return
    started = true
    stops = [
      onEvent<SyncUpdatedPayload>(EVENTS.SYNC_UPDATED, onSyncUpdated),
      onEvent<NotifyOpenThreadPayload>(EVENTS.NOTIFY_OPEN_THREAD, (p) => void onNotifyOpen(p)),
      watch(
        () => inbox.selectedRef,
        (ref) => {
          if (ref) void open(ref)
          else close()
        },
      ),
    ]
  }

  function stop(): void {
    for (const s of stops) s()
    stops = []
    started = false
  }

  return {
    current,
    currentRef,
    loading,
    error,
    unread,
    ...extras,
    open,
    close,
    reload,
    archive,
    trash,
    toggleRead,
    start,
    stop,
  }
})
