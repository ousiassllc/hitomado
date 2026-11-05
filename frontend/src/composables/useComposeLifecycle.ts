// 作成セッションの保存・終了（composeStore の一部）。下書き保存は自動保存（useAutosave）に集約する。
import type { Ref } from 'vue'
import { ComposeService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  ComposeContent,
  ComposeSession,
  SendPreview,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, parseBindingError } from '../lib/api'
import { type EditableContent, snapshotOf } from '../lib/composeModel'
import { compose as msg } from '../lib/messages'
import { useUiStore } from '../stores/ui'
import type { Autosave } from './useAutosave'

export interface ComposeLifecycleContext {
  session: Ref<ComposeSession | undefined>
  preview: Ref<SendPreview | undefined>
  dirty: Ref<boolean>
  sending: Ref<boolean>
  error: Ref<string | undefined>
  saveError: Ref<string | undefined>
  content: Ref<EditableContent>
  autosave: Autosave<ComposeContent>
  clearSession: () => void
  /** エラーの共通効果（オフライン・要再接続）を反映し、表示文言を返す。 */
  fail: (e: unknown) => string
}

export function useComposeLifecycle(ctx: ComposeLifecycleContext) {
  const ui = useUiStore()

  /** 手動保存。送信元未選択ではメモリ上のみのため保存できない。 */
  async function save(): Promise<boolean> {
    if (!ctx.session.value) return false
    if (ctx.session.value.accountId === undefined) {
      ctx.error.value = msg.errors.SENDER_REQUIRED
      return false
    }
    ctx.dirty.value = true
    ctx.autosave.notify(snapshotOf(ctx.content.value))
    return ctx.autosave.flush()
  }

  /** 未保存があれば直ちに保存して完了を待つ（アプリ終了前など）。送信元未選択は保存対象外。 */
  async function flush(): Promise<boolean> {
    if (!ctx.session.value || ctx.session.value.accountId === undefined) return true
    return ctx.autosave.flush()
  }

  /** 保存してから閉じる。送信元未選択はメモリのみなので保存せず閉じる。保存に失敗したら閉じない。 */
  async function close(): Promise<boolean> {
    const s = ctx.session.value
    if (!s || ctx.sending.value) return false
    ctx.preview.value = undefined
    if (s.accountId !== undefined) {
      if (!(await ctx.autosave.flush())) {
        ctx.error.value = ctx.saveError.value
          ? `${msg.closeFailed}（${ctx.saveError.value}）`
          : msg.closeFailed
        return false
      }
    } else {
      await ctx.autosave.drain()
    }
    try {
      await call(ComposeService.CloseCompose(s.sessionId))
    } catch (e) {
      if (parseBindingError(e).code !== 'SESSION_NOT_FOUND') ui.pushAlert(ctx.fail(e))
    }
    ctx.clearSession()
    return true
  }

  /** セッションと Gmail の下書きを破棄する。失敗したら閉じない。 */
  async function discard(): Promise<boolean> {
    const s = ctx.session.value
    if (!s || ctx.sending.value) return false
    await ctx.autosave.drain()
    try {
      await call(ComposeService.DiscardCompose(s.sessionId))
    } catch (e) {
      if (parseBindingError(e).code !== 'SESSION_NOT_FOUND') {
        ctx.error.value = ctx.fail(e) || msg.discardFailed
        return false
      }
    }
    ctx.clearSession()
    return true
  }

  return { save, flush, close, discard }
}
