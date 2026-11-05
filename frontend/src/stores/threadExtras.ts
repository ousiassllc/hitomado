// スレッド表示に付随する操作（ラベル・添付の保存・リンクを開く）。threadStore から使う。
import { computed, type Ref, ref } from 'vue'
import type {
  Label,
  ThreadDetail,
  ThreadRef,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import * as ThreadService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/threadservice'
import type { AttachmentInfo } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { type AppError, list } from '../lib/api'
import { thread as msg } from '../lib/messages'
import { isOpenableUrl } from '../lib/sanitize'
import type { useInboxStore } from './inbox'
import type { useUiStore } from './ui'

export interface ThreadExtrasDeps {
  currentRef: Ref<ThreadRef | undefined>
  current: Ref<ThreadDetail | undefined>
  ui: ReturnType<typeof useUiStore>
  inbox: ReturnType<typeof useInboxStore>
  api: <T>(p: PromiseLike<T>) => Promise<T>
  reload: () => Promise<void>
}

export function useThreadExtras(d: ThreadExtrasDeps) {
  /** アカウントごとのユーザーラベル（ラベルメニューを開いたときに 1 度だけ取得） */
  const labelsByAccount = ref<Record<string, Label[]>>({})
  const labelsLoading = ref(false)
  const accountLabels = computed<Label[]>(
    () => labelsByAccount.value[d.currentRef.value?.accountId ?? ''] ?? [],
  )

  /** ラベルメニュー用に、受信アカウントのラベル一覧を取得する。 */
  async function loadLabels(): Promise<void> {
    const ref = d.currentRef.value
    if (!ref || labelsLoading.value || ref.accountId in labelsByAccount.value) return
    labelsLoading.value = true
    try {
      const ls = list(await d.api(InboxService.ListLabels(ref.accountId)))
      labelsByAccount.value = { ...labelsByAccount.value, [ref.accountId]: ls }
    } catch {
      d.ui.pushAlert(msg.labelsLoadFailed)
    } finally {
      labelsLoading.value = false
    }
  }

  /** ラベルの付与・除去を切り替える。一覧の行は inboxStore が差し替え、成功したら表示中のスレッドも更新する。 */
  async function toggleLabel(labelId: string): Promise<boolean> {
    const ref = d.currentRef.value
    const cur = d.current.value
    if (!ref || !cur) return false
    const has = cur.labels?.some((l) => l.id === labelId) ?? false
    const ok = await d.inbox.modifyLabel([ref], labelId, !has)
    if (ok) await d.reload()
    return ok
  }

  /** 添付を保存する。保存先ダイアログのキャンセル（saved=false）は何もしない。 */
  async function saveAttachment(messageId: string, a: AttachmentInfo): Promise<void> {
    const ref = d.currentRef.value
    if (!ref) return
    try {
      const r = await d.api(ThreadService.SaveAttachment({ ...ref, messageId }, a.id, a.filename))
      if (r.saved) d.ui.pushToast('success', msg.attachmentSaved)
    } catch (e) {
      d.ui.pushAlert(`${msg.attachmentFailed}${(e as AppError).message ?? ''}`)
    }
  }

  /** 本文中のリンクをシステムブラウザ／メーラーで開く。許可スキーム（http／https／mailto）以外は呼ばない。 */
  async function openExternal(url: string): Promise<void> {
    if (!isOpenableUrl(url)) return
    try {
      await d.api(ThreadService.OpenExternal(url))
    } catch (e) {
      d.ui.pushAlert(`${msg.openFailed}${(e as AppError).message ?? ''}`)
    }
  }

  return { accountLabels, labelsLoading, loadLabels, toggleLabel, saveAttachment, openExternal }
}
