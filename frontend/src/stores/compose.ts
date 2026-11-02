import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { ComposeService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  ComposeAttachment,
  ComposeContent,
  ComposeSession,
  OpenComposeRequest,
  SendPreview,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useAutosave } from '../composables/useAutosave'
import { useComposeLifecycle } from '../composables/useComposeLifecycle'
import { useComposeSend } from '../composables/useComposeSend'
import { useSignatureSwap } from '../composables/useSignatureSwap'
import { call, list, parseBindingError } from '../lib/api'
import { describeComposeError, draftAttachmentWarning } from '../lib/composeErrors'
import {
  type EditableContent,
  emptyContent,
  normalizeContent,
  normalizeSession,
  snapshotOf,
  type ThreadTarget,
  withDefaultSender,
} from '../lib/composeModel'
import { applyErrorEffects } from '../lib/errorEffects'
import { compose as msg } from '../lib/messages'
import { cleanRichHtml } from '../lib/richtext'
import { replaceUserBody } from '../lib/signature'
import { useAccountsStore } from './accounts'
import { useInboxStore } from './inbox'
import { useUiStore } from './ui'

export type { EditableContent, ThreadTarget } from '../lib/composeModel'

export const useComposeStore = defineStore('compose', () => {
  const ui = useUiStore()
  const accounts = useAccountsStore()
  const inbox = useInboxStore()

  const session = ref<ComposeSession>()
  const openRef = ref<ThreadTarget>()
  const content = ref<EditableContent>(emptyContent())
  const attachments = ref<ComposeAttachment[]>([])
  const preview = ref<SendPreview>()
  const previewArchive = ref(false)
  const dirty = ref(false)
  const saving = ref(false)
  const preparing = ref(false)
  const sending = ref(false)
  /** AI 下書きの生成中。本文エディタを読み取り専用にする（生成中の入力が確認なしで置換されるのを防ぐ）。 */
  const aiGenerating = ref(false)
  /** 送信・確認・セッション操作のエラー（エディタ上部に表示）。 */
  const error = ref<string>()
  const saveError = ref<string>()
  const attachmentError = ref<string>()

  /** 内容の版。非同期の応答が返った時点で版が変わっていれば、その応答（確認など）は捨てる。 */
  let version = 0
  let silent = 0
  let opening = false

  const isNew = computed(() => session.value?.mode === 'new')
  const canArchive = computed(() => !!session.value && session.value.mode !== 'new')
  const canSend = computed(
    () => session.value?.accountId !== undefined && !sending.value && !preparing.value,
  )

  function isOpenForThread(ref: ThreadTarget): boolean {
    const r = openRef.value
    return !!session.value && r?.accountId === ref.accountId && r.threadId === ref.threadId
  }

  /** 状態変更を通知なし（dirty・自動保存・確認破棄を起こさない）で行う。 */
  function silently(fn: () => void): void {
    silent++
    try {
      fn()
    } finally {
      silent--
    }
  }

  function fail(e: unknown): string {
    const err = parseBindingError(e)
    applyErrorEffects(err, { setOnline: ui.setOnline, markNeedsReauth: accounts.markNeedsReauth })
    return describeComposeError(err)
  }

  const autosave = useAutosave({
    canSave: () => session.value?.accountId !== undefined && !sending.value,
    save: async (snap: ComposeContent) => {
      const sid = session.value?.sessionId
      if (sid === undefined) throw new Error('no session')
      saving.value = true
      try {
        const r = await call(ComposeService.SaveDraft(sid, snap))
        if (!ui.online) ui.setOnline(true)
        return r
      } finally {
        saving.value = false
      }
    },
    onSaved: (r) => {
      dirty.value = false
      saveError.value = undefined
      if (session.value) session.value = { ...session.value, draftId: r.draftId }
    },
    onError: (e) => {
      saveError.value = fail(e)
    },
  })

  /** 編集が入った。確認済みの内容は無効にし、未保存として自動保存へ知らせる。 */
  function onEdited(): void {
    if (silent > 0 || !session.value) return
    version++
    dirty.value = true
    preview.value = undefined
    autosave.notify(snapshotOf(content.value))
  }
  watch(content, onEdited, { deep: true, flush: 'sync' })
  watch(attachments, onEdited, { flush: 'sync' })
  watch(preview, (p) => (p ? ui.openDialog('sendConfirm') : ui.closeDialog('sendConfirm')), {
    flush: 'sync',
  })

  const sigSwap = useSignatureSwap({ session, content, accounts })

  /** 署名の差し替えは利用者の編集ではないため dirty にしない（署名だけの空下書きを作らない）。 */
  function swapSignature(fn: () => void): void {
    silently(fn)
    version++
    preview.value = undefined
    if (dirty.value) autosave.notify(snapshotOf(content.value))
  }

  function clearSession(): void {
    silently(() => {
      session.value = undefined
      openRef.value = undefined
      content.value = emptyContent()
      attachments.value = []
    })
    version++
    preview.value = undefined
    previewArchive.value = false
    dirty.value = false
    error.value = undefined
    saveError.value = undefined
    attachmentError.value = undefined
    aiGenerating.value = false
    sigSwap.reset()
  }

  function sameTarget(req: OpenComposeRequest): boolean {
    const s = session.value
    if (!s || s.mode !== req.mode || req.draftId !== undefined) return false
    if (req.mode === 'new') return true
    return (
      openRef.value?.accountId === req.ref?.accountId &&
      openRef.value?.threadId === req.ref?.threadId
    )
  }

  /** 作成セッションを開く。別セッションが開いていれば保存して閉じてから。開けたら（または同じものが開いていれば）true。 */
  async function open(req: OpenComposeRequest): Promise<boolean> {
    if (opening) return false
    if (sameTarget(req)) return true
    opening = true
    try {
      if (session.value && !(await close())) return false
      const s = normalizeSession(
        await call(ComposeService.OpenCompose(withDefaultSender(req, inbox.filter.accountIds))),
      )
      silently(() => {
        session.value = s
        openRef.value = req.ref
          ? { accountId: req.ref.accountId, threadId: req.ref.threadId }
          : undefined
        content.value = normalizeContent(s.content)
        attachments.value = list(s.attachments)
      })
      sigSwap.init()
      const warning = draftAttachmentWarning(s)
      if (warning) ui.pushToast('error', warning)
      return true
    } catch (e) {
      ui.pushToast('error', fail(e) || msg.openFailed)
      return false
    } finally {
      opening = false
    }
  }

  /** 新規作成のみ。SetComposeSender の後、署名を差し替える（編集済みなら確認待ち `signatureConflict`）。 */
  async function setSender(accountId: string): Promise<boolean> {
    const s = session.value
    if (!s || s.senderLocked || s.mode !== 'new' || sending.value) return false
    if (s.accountId === accountId) return true
    error.value = undefined
    try {
      const updated = normalizeSession(
        await call(ComposeService.SetComposeSender(s.sessionId, accountId)),
      )
      if (session.value?.sessionId !== s.sessionId) return false
      session.value = { ...updated, content: s.content }
    } catch (e) {
      error.value = fail(e)
      return false
    }
    swapSignature(() => sigSwap.onSenderChanged(accountId))
    return true
  }

  function resolveSignatureConflict(replace: boolean): void {
    swapSignature(() => sigSwap.resolve(replace))
  }

  const { save, flush, close, discard } = useComposeLifecycle({
    session,
    preview,
    dirty,
    sending,
    error,
    saveError,
    content,
    autosave,
    clearSession,
    fail,
  })

  const send = useComposeSend({
    session,
    openRef,
    content,
    attachments,
    preview,
    previewArchive,
    dirty,
    preparing,
    sending,
    error,
    attachmentError,
    canArchive,
    version: () => version,
    drainAutosave: () => autosave.drain(),
    resaveIfDirty: () => {
      if (dirty.value) autosave.notify(snapshotOf(content.value))
    },
    clearSession,
    fail,
  })
  const { prepareSend, cancelPreview, confirmSend, addAttachments, removeAttachment } = send

  /** AI 下書きの結果を、署名・引用を保ったまま本文（署名より前）に入れる。 */
  function insertGenerated(bodyHtml: string): void {
    if (session.value)
      content.value.bodyHtml = replaceUserBody(content.value.bodyHtml, cleanRichHtml(bodyHtml))
  }

  return {
    session,
    content,
    attachments,
    preview,
    previewArchive,
    dirty,
    saving,
    preparing,
    sending,
    aiGenerating,
    error,
    saveError,
    attachmentError,
    signatureConflict: sigSwap.conflict,
    isNew,
    canArchive,
    canSend,
    isOpenForThread,
    open,
    setSender,
    resolveSignatureConflict,
    save,
    flush,
    close,
    discard,
    prepareSend,
    cancelPreview,
    confirmSend,
    addAttachments,
    removeAttachment,
    insertGenerated,
  }
})
