// 作成・送信のエラーコード別の表示文言と、送信確認の扱い（バインディング仕様 §1.2、§5.2）。
import { type AppError, parseBindingError } from './api'
import { formatSize } from './format'
import { compose as msg } from './messages'

/** 確認トークンの再取得が必要になるコード（再度 prepareSend へ誘導する）。 */
const NEEDS_REPREPARE = ['CONFIRM_REQUIRED', 'CONFIRM_EXPIRED', 'CONFIRM_MISMATCH'] as const

export function needsReprepare(code: string): boolean {
  return (NEEDS_REPREPARE as readonly string[]).includes(code)
}

function emailsOf(details: Record<string, unknown> | undefined): string[] {
  const v = details?.emails
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}

/** 上限バイト数（details.limitBytes）。無ければ Gmail の 25MB。 */
function limitLabel(details: Record<string, unknown> | undefined): string {
  const v = details?.limitBytes
  return formatSize(typeof v === 'number' && v > 0 ? v : 25 * 1024 * 1024)
}

/**
 * OpenCompose の結果に下書き添付の復元失敗（`attachmentError`）があれば、警告文言を返す。
 * 本文・宛先は復元済みで attachments は空（Go 側の仕様）。
 */
// バインディング再生成前後どちらの ComposeSession 型でも渡せるよう、緩い型で受ける。
export function draftAttachmentWarning(session: object): string | undefined {
  const e = (session as { attachmentError?: unknown }).attachmentError
  if (e === undefined || e === null) return undefined
  const err = parseBindingError(e)
  return err.code === 'ATTACHMENT_TOO_LARGE'
    ? msg.toast.draftAttachmentsDropped(limitLabel(err.details))
    : msg.toast.draftAttachmentsFailed
}

/**
 * 利用者に見せる日本語メッセージ。仕様で定めたコードは固定文言（再度の確認への誘導を含む）、
 * それ以外（OFFLINE・GMAIL_ERROR 等）は Go が返した message をそのまま使う。
 */
export function describeComposeError(e: AppError): string {
  switch (e.code) {
    case 'SENDER_REQUIRED':
    case 'NO_RECIPIENTS':
    case 'SENDER_MISMATCH':
    case 'SENDER_LOCKED':
    case 'SESSION_NOT_FOUND':
    case 'CONFIRM_REQUIRED':
    case 'CONFIRM_EXPIRED':
    case 'CONFIRM_MISMATCH':
      return msg.errors[e.code]
    case 'INVALID_ADDRESS':
      return msg.errors.INVALID_ADDRESS(emailsOf(e.details))
    case 'ATTACHMENT_TOO_LARGE':
      return msg.errors.ATTACHMENT_TOO_LARGE(limitLabel(e.details))
    default:
      return e.message || msg.errors.sendFailed
  }
}
