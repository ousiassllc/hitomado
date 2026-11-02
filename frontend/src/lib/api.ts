// バインディング呼び出しの共通ラッパー。Go 側が返すエラー（apperr の JSON）を AppError に統一する。

export type ErrorCode =
  | 'VALIDATION'
  | 'NOT_FOUND'
  | 'SESSION_NOT_FOUND'
  | 'OFFLINE'
  | 'RATE_LIMITED'
  | 'GMAIL_ERROR'
  | 'NEEDS_REAUTH'
  | 'ACCOUNT_DUPLICATE'
  | 'OAUTH_CLIENT_IN_USE'
  | 'OAUTH_CANCELLED'
  | 'OAUTH_DENIED'
  | 'OAUTH_TIMEOUT'
  | 'SENDER_REQUIRED'
  | 'SENDER_LOCKED'
  | 'SENDER_MISMATCH'
  | 'NO_RECIPIENTS'
  | 'INVALID_ADDRESS'
  | 'ATTACHMENT_TOO_LARGE'
  | 'ATTACHMENT_IO'
  | 'CONFIRM_REQUIRED'
  | 'CONFIRM_EXPIRED'
  | 'CONFIRM_MISMATCH'
  | 'AI_NOT_CONFIGURED'
  | 'AI_CONSENT_REQUIRED'
  | 'AI_FAILED'
  | 'CANCELLED'
  | 'INTERNAL'

export interface AppError {
  code: ErrorCode
  message: string
  retryable: boolean
  accountId?: string
  details?: Record<string, unknown>
}

const FALLBACK_MESSAGE = '予期しないエラーが発生しました。'

function isAppError(v: unknown): v is AppError {
  return (
    typeof v === 'object' &&
    v !== null &&
    typeof (v as AppError).code === 'string' &&
    typeof (v as AppError).message === 'string'
  )
}

/**
 * reject 値（Wails の RuntimeError。error.cause に apperr の JSON が入る）を AppError に変換する。
 * 解釈できない値は INTERNAL にする。
 */
export function parseBindingError(e: unknown): AppError {
  if (isAppError(e)) return { ...e, retryable: e.retryable === true }
  const cause = (e as { cause?: unknown } | null)?.cause
  const candidates: unknown[] = [cause]
  const message = (e as { message?: unknown } | null)?.message
  if (typeof message === 'string') candidates.push(message)
  if (typeof e === 'string') candidates.push(e)
  for (const c of candidates) {
    const parsed = typeof c === 'string' ? tryParse(c) : c
    if (isAppError(parsed)) return { ...parsed, retryable: parsed.retryable === true }
  }
  return { code: 'INTERNAL', message: FALLBACK_MESSAGE, retryable: false }
}

function tryParse(s: string): unknown {
  try {
    return JSON.parse(s)
  } catch {
    return undefined
  }
}

/** バインディング呼び出しを実行し、reject を AppError にして投げ直す。キャンセル可能性は保持しない。 */
export async function call<T>(p: PromiseLike<T>): Promise<T> {
  try {
    return await p
  } catch (e) {
    throw parseBindingError(e)
  }
}

export function isAppErrorLike(e: unknown): e is AppError {
  return isAppError(e)
}

/** null（Go の nil スライス）を空配列に正規化する。 */
export function list<T>(v: T[] | null | undefined): T[] {
  return v ?? []
}
