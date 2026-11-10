// 作成セッションの送信フローと添付（composeStore の一部）。
// 送信は必ず PrepareSend（確認トークン）→ 確認ダイアログ → Send の順。確認後に内容が変わったら preview は破棄される
// （composeStore の watch）。失敗してもセッション・本文・添付は保持し、再度 prepareSend から送れる。
// 設定「送信前に確認画面を出す」がオフで、警告が無いときだけ、ダイアログを出さずに PrepareSend の確認トークンで
// そのまま Send する（#313。送信元の固定と、送信ボタンを押したときだけ送ることは変えない）。
import type { ComputedRef, Ref } from 'vue'
import { ComposeService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  ComposeAttachment,
  ComposeSession,
  SendPreview,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../lib/api'
import { needsReprepare } from '../lib/composeErrors'
import {
  type EditableContent,
  normalizePreview,
  snapshotOf,
  type ThreadTarget,
} from '../lib/composeModel'
import { compose as msg } from '../lib/messages'
import { useInboxStore } from '../stores/inbox'
import { useSettingsStore } from '../stores/settings'
import { useUiStore } from '../stores/ui'

export interface ComposeSendContext {
  session: Ref<ComposeSession | undefined>
  openRef: Ref<ThreadTarget | undefined>
  content: Ref<EditableContent>
  attachments: Ref<ComposeAttachment[]>
  preview: Ref<SendPreview | undefined>
  previewArchive: Ref<boolean>
  dirty: Ref<boolean>
  preparing: Ref<boolean>
  sending: Ref<boolean>
  error: Ref<string | undefined>
  attachmentError: Ref<string | undefined>
  canArchive: ComputedRef<boolean>
  /** 内容の版。応答が返った時点で版が変わっていれば、その応答は捨てる。 */
  version: () => number
  drainAutosave: () => Promise<void>
  resaveIfDirty: () => void
  clearSession: () => void
  /** エラーの共通効果（オフライン・要再接続）を反映し、表示文言を返す。 */
  fail: (e: unknown) => string
}

export function useComposeSend(ctx: ComposeSendContext) {
  const ui = useUiStore()
  const inbox = useInboxStore()
  const settings = useSettingsStore()

  /**
   * 確認画面を省いてよいか。設定がオフ（読み込み前・不明は「出す」）で、警告が 1 件も無いときだけ。
   * 警告（OTHER_OWN_ACCOUNT・EMPTY_SUBJECT・EMPTY_BODY）が 1 件でもあるときは、種類によらず、設定にかかわらず確認画面を出す（#313。仕様で確定）。
   */
  function canSkipConfirm(p: SendPreview): boolean {
    return settings.appInfo?.sendConfirmEnabled === false && !p.warnings?.length
  }

  /** 送信前確認を取得する。成功すると `preview` が入り、SendConfirmDialog が開く（確認を省く設定のときは、そのまま送信する）。 */
  async function prepareSend(archive: boolean): Promise<boolean> {
    const s = ctx.session.value
    if (!s || ctx.sending.value || ctx.preparing.value) return false
    ctx.error.value = undefined
    if (s.accountId === undefined) {
      ctx.error.value = msg.errors.SENDER_REQUIRED
      return false
    }
    const wantArchive = archive && ctx.canArchive.value
    const at = ctx.version()
    let skip: SendPreview | undefined
    ctx.preparing.value = true
    try {
      const p = await call(
        ComposeService.PrepareSend(s.sessionId, snapshotOf(ctx.content.value), {
          archiveAfterSend: wantArchive,
        }),
      )
      if (ctx.session.value?.sessionId !== s.sessionId) return false
      if (at !== ctx.version()) {
        ctx.error.value = msg.errors.changedWhilePreparing
        return false
      }
      ctx.previewArchive.value = wantArchive
      const next = normalizePreview(p)
      if (!canSkipConfirm(next)) {
        ctx.preview.value = next
        return true
      }
      skip = next
    } catch (e) {
      ctx.error.value = ctx.fail(e)
      return false
    } finally {
      ctx.preparing.value = false
    }
    return skip ? sendPrepared(s, skip) : false
  }

  /** 確認ダイアログのキャンセル。トークンを破棄してエディタへ戻る（再度 prepareSend が必要）。 */
  function cancelPreview(): void {
    ctx.preview.value = undefined
  }

  /** 送信後の受信箱の反映。アーカイブしたスレッドは一覧から外し、先頭ページを取り直す。 */
  function reflectInInbox(ref: ThreadTarget | undefined, archived: boolean): void {
    if (!ref) return
    if (archived && inbox.mode === 'inbox') inbox.removeThread(ref)
    inbox.onSyncUpdated({ accountId: ref.accountId, changedThreadIds: [ref.threadId] })
  }

  /** 確認ダイアログで確認済みの内容を送信する。 */
  function confirmSend(): Promise<boolean> {
    const s = ctx.session.value
    const p = ctx.preview.value
    return s && p ? sendPrepared(s, p) : Promise.resolve(false)
  }

  /** 確認トークンを持つ内容を送信する。失敗してもセッション・本文・添付は保持し、再度 prepareSend から送れる。 */
  async function sendPrepared(s: ComposeSession, p: SendPreview): Promise<boolean> {
    if (ctx.sending.value) return false
    ctx.sending.value = true
    ctx.error.value = undefined
    try {
      await ctx.drainAutosave() // 送信後に下書きが作り直されないよう、実行中の保存を待つ
      const result = await call(
        ComposeService.Send(s.sessionId, p.confirmToken, snapshotOf(ctx.content.value)),
      )
      if (!ui.online) ui.setOnline(true)
      const archive = ctx.previewArchive.value
      const ref = ctx.openRef.value
      ctx.clearSession()
      ui.pushToast('success', archive && result.archived ? msg.toast.sentArchived : msg.toast.sent)
      if (archive && !result.archived) ui.pushAlert(msg.toast.archiveFailed)
      reflectInInbox(ref, archive && result.archived)
      return true
    } catch (e) {
      // 確認トークンは使用済み・無効のため確認をやり直す。本文・添付・宛先は保持する。
      ctx.preview.value = undefined
      const text = ctx.fail(e)
      // SEND_UNCERTAIN は送信済みの可能性があるため「送信に失敗しました」を前置しない。
      const code = parseBindingError(e).code
      ctx.error.value =
        needsReprepare(code) || code === 'SEND_UNCERTAIN'
          ? text
          : `${msg.errors.sendFailed} ${text}`
      ctx.resaveIfDirty()
      return false
    } finally {
      ctx.sending.value = false
    }
  }

  async function changeAttachments(
    op: (sessionId: string) => PromiseLike<ComposeAttachment[] | null>,
  ): Promise<boolean> {
    const s = ctx.session.value
    if (!s || ctx.sending.value) return false
    ctx.attachmentError.value = undefined
    try {
      const next = list(await call(op(s.sessionId)))
      if (ctx.session.value?.sessionId === s.sessionId) ctx.attachments.value = next
      return true
    } catch (e) {
      ctx.attachmentError.value = ctx.fail(e)
      return false
    }
  }

  const addAttachments = (paths: string[]) =>
    paths.length === 0
      ? Promise.resolve(false)
      : changeAttachments((sid) => ComposeService.AddAttachments(sid, paths))
  const removeAttachment = (id: string) =>
    changeAttachments((sid) => ComposeService.RemoveAttachment(sid, id))

  return { prepareSend, cancelPreview, confirmSend, addAttachments, removeAttachment }
}
