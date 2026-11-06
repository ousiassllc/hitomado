// バインディング呼び出しの共通ラッパー。Go 側が返すエラー（apperr の JSON）を AppError に統一する。

export const ERROR_CODES = [
  'VALIDATION',
  'NOT_FOUND',
  'SESSION_NOT_FOUND',
  'OFFLINE',
  'RATE_LIMITED',
  'GMAIL_ERROR',
  'NEEDS_REAUTH',
  'OAUTH_CLIENT_REJECTED',
  'ACCOUNT_DUPLICATE',
  'OAUTH_CLIENT_IN_USE',
  'OAUTH_CANCELLED',
  'OAUTH_DENIED',
  'OAUTH_TIMEOUT',
  'SENDER_REQUIRED',
  'SENDER_LOCKED',
  'SENDER_MISMATCH',
  'NO_RECIPIENTS',
  'INVALID_ADDRESS',
  'ATTACHMENT_TOO_LARGE',
  'ATTACHMENT_IO',
  'CONFIRM_REQUIRED',
  'CONFIRM_EXPIRED',
  'CONFIRM_MISMATCH',
  'SEND_UNCERTAIN',
  'AI_NOT_CONFIGURED',
  'AI_CONSENT_REQUIRED',
  'AI_FAILED',
  'CANCELLED',
  'UNSUPPORTED',
  'INTERNAL',
] as const

export type ErrorCode = (typeof ERROR_CODES)[number]

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
  if (typeof message === 'string') {
    const fromText = parseBoundMethodMessage(message)
    if (fromText) return fromText
  }
  return { code: 'INTERNAL', message: FALLBACK_MESSAGE, retryable: false }
}

const ERROR_CODE_SET: ReadonlySet<string> = new Set(ERROR_CODES)
const RETRYABLE_CODES: ReadonlySet<string> = new Set(['RATE_LIMITED', 'OFFLINE', 'OAUTH_TIMEOUT'])
const BOUND_METHOD_ERROR = 'Bound method returned an error: '

/**
 * Android の Wails の転送路は、Go のエラーの cause（apperr の JSON）を捨て、メッセージの文字列
 * （"Binding call failed: Bound method returned an error: <CODE>: <利用者向けの文面>"）だけを JS へ返す。
 * そのままだと全てのエラーが INTERNAL の定型文になるので、既知のコードで始まる文面だけ取り出して AppError に戻す。
 * details・accountId は転送路で失われるので復元できない（コードと文面だけ）。
 */
function parseBoundMethodMessage(text: string): AppError | undefined {
  const i = text.indexOf(BOUND_METHOD_ERROR)
  if (i < 0) return undefined
  const m = /^([A-Z][A-Z_]*): ([^\n]+)/.exec(text.slice(i + BOUND_METHOD_ERROR.length))
  if (!m || !ERROR_CODE_SET.has(m[1])) return undefined
  return { code: m[1] as ErrorCode, message: m[2], retryable: RETRYABLE_CODES.has(m[1]) }
}

function tryParse(s: string): unknown {
  try {
    return JSON.parse(s)
  } catch {
    return undefined
  }
}

let bindingErrorReporter: ((e: AppError) => void) | undefined

/** バインディング呼び出しの失敗を記録する先（Go のログへ送る処理。errorReporter が取り付ける）。 */
export function setBindingErrorReporter(fn: ((e: AppError) => void) | undefined): void {
  bindingErrorReporter = fn
}

/** バインディング呼び出しを実行し、reject を AppError にして投げ直す（失敗はログへも報告する）。キャンセル可能性は保持しない。 */
export async function call<T>(p: PromiseLike<T>): Promise<T> {
  try {
    return await p
  } catch (e) {
    const err = parseBindingError(e)
    bindingErrorReporter?.(err)
    throw err
  }
}

export function isAppErrorLike(e: unknown): e is AppError {
  return isAppError(e)
}

/** null（Go の nil スライス）を空配列に正規化する。 */
export function list<T>(v: T[] | null | undefined): T[] {
  return v ?? []
}
