import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import type {
  Label,
  ThreadDetail,
  ThreadRef,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import * as ThreadService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/threadservice'
import type { AttachmentInfo } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
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
import { isOpenableUrl } from '../lib/sanitize'
import { useAccountsStore } from './accounts'
import { useInboxStore } from './inbox'
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
  /** アカウントごとのユーザーラベル（ラベルメニューを開いたときに 1 度だけ取得） */
  const labelsByAccount = ref<Record<string, Label[]>>({})
  const labelsLoading = ref(false)

  let seq = 0
  /** 既読化・未読化の呼び出し中のスレッドキー（スレッド単位。別スレッドの表示をブロックしない） */
  const marking = new Set<string>()
  /**
   * 表示中スレッドで自動既読化を済ませた（または既読だった）メッセージ ID。再取得で増えた未読だけを自動既読に
   * する（手動で「未読にする」を戻さない）。未読メッセージは既読化の確定後にだけ追加する。
   */
  const seenIds = new Set<string>()
  let started = false
  let stops: (() => void)[] = []

  const accountLabels = computed<Label[]>(
    () => labelsByAccount.value[currentRef.value?.accountId ?? ''] ?? [],
  )
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
  }

  /** 表示後に未読があれば既読にする（inboxStore 経由。失敗時のトーストと復元は inboxStore が行う）。 */
  async function autoMarkRead(ref: ThreadRef, ids: string[]): Promise<void> {
    const key = threadKey(ref.accountId, ref.threadId)
    marking.add(key)
    try {
      const ok = await inbox.markRead([ref], true)
      // 表示が別スレッドに移っていたら（seenIds は表示中スレッドのもの）何も反映しない
      if (ok && currentKey.value === key) {
        for (const id of ids) seenIds.add(id)
        unread.value = false
      }
    } finally {
      marking.delete(key)
    }
  }

  async function load(ref: ThreadRef): Promise<void> {
    const my = ++seq
    const key = threadKey(ref.accountId, ref.threadId)
    if (!current.value) loading.value = true
    try {
      const d = normalize(await api(ThreadService.GetThread(ref)))
      if (my !== seq) return
      const msgs = list(d.messages)
      for (const m of msgs) if (!m.isUnread) seenIds.add(m.id)
      const fresh = msgs.filter((m) => m.isUnread && !seenIds.has(m.id)).map((m) => m.id)
      const busy = marking.has(key)
      current.value = d
      error.value = undefined
      if (!busy) unread.value = msgs.some((m) => m.isUnread)
      if (fresh.length > 0 && !busy) void autoMarkRead(ref, fresh)
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
    marking.add(key)
    try {
      const ok = await inbox.markRead([ref], toRead)
      if (ok && currentKey.value === key) unread.value = !toRead
      return ok
    } finally {
      marking.delete(key)
    }
  }

  /** ラベルメニュー用に、受信アカウントのラベル一覧を取得する。 */
  async function loadLabels(): Promise<void> {
    const ref = currentRef.value
    if (!ref || labelsLoading.value || ref.accountId in labelsByAccount.value) return
    labelsLoading.value = true
    try {
      const ls = list(await api(InboxService.ListLabels(ref.accountId)))
      labelsByAccount.value = { ...labelsByAccount.value, [ref.accountId]: ls }
    } catch {
      ui.pushToast('error', msg.labelsLoadFailed)
    } finally {
      labelsLoading.value = false
    }
  }

  /** ラベルの付与・除去を切り替える。一覧の行は inboxStore が差し替え、成功したら表示中のスレッドも更新する。 */
  async function toggleLabel(labelId: string): Promise<boolean> {
    const ref = currentRef.value
    const d = current.value
    if (!ref || !d) return false
    const has = d.labels?.some((l) => l.id === labelId) ?? false
    const ok = await inbox.modifyLabel([ref], labelId, !has)
    if (ok) await reload()
    return ok
  }

  /** 添付を保存する。保存先ダイアログのキャンセル（saved=false）は何もしない。 */
  async function saveAttachment(messageId: string, a: AttachmentInfo): Promise<void> {
    const ref = currentRef.value
    if (!ref) return
    try {
      const r = await api(ThreadService.SaveAttachment({ ...ref, messageId }, a.id, a.filename))
      if (r.saved) ui.pushToast('success', msg.attachmentSaved)
    } catch (e) {
      ui.pushToast('error', `${msg.attachmentFailed}${(e as AppError).message ?? ''}`)
    }
  }

  /** 本文中のリンクをシステムブラウザ／メーラーで開く。許可スキーム（http／https／mailto）以外は呼ばない。 */
  async function openExternal(url: string): Promise<void> {
    if (!isOpenableUrl(url)) return
    try {
      await api(ThreadService.OpenExternal(url))
    } catch (e) {
      ui.pushToast('error', `${msg.openFailed}${(e as AppError).message ?? ''}`)
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
    accountLabels,
    labelsLoading,
    open,
    close,
    reload,
    archive,
    trash,
    toggleRead,
    loadLabels,
    toggleLabel,
    saveAttachment,
    openExternal,
    start,
    stop,
  }
})
