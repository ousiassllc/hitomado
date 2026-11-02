// フロントのエラー（未処理の例外・Promise の reject・Vue の描画エラー・バインディング呼び出しの失敗）を Go 側のログへ送る。
// 送るのは種類・メッセージ・スタック・発生元・エラーコード・アカウント ID だけで、メール本文・件名・宛先を載せる経路は無い。
// メッセージに混ざった秘密の伏せ処理は Go 側のログ出力（logging.Redact）が行う。
import type { App } from 'vue'
import { LogService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type { ClientError } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { type AppError, setBindingErrorReporter } from './api'

/** 画面側で送る文字数の上限（Go 側でも切る）。 */
const MAX_MESSAGE = 1000
const MAX_STACK = 8000

/** 利用者の取り消し操作は障害ではないので記録しない。 */
const IGNORED_CODES: ReadonlySet<string> = new Set(['CANCELLED', 'OAUTH_CANCELLED'])

export type SendClientError = (e: ClientError) => Promise<void>

const defaultSend: SendClientError = (e) => LogService.ReportClientError(e)

function clip(s: string | undefined, max: number): string {
  if (!s) return ''
  return s.length > max ? s.slice(0, max) : s
}

/** 例外値（Error・文字列・その他）から message と stack を取り出す。オブジェクトの中身は文字列化しない。 */
export function describeThrown(v: unknown): { message: string; stack: string } {
  if (v instanceof Error) return { message: v.message, stack: v.stack ?? '' }
  if (typeof v === 'string') return { message: v, stack: '' }
  return { message: `non-error value (${typeof v})`, stack: '' }
}

export interface ErrorReporter {
  report(e: ClientError): void
  /** window・Vue のハンドラを取り付ける（1 回だけ呼ぶ）。戻り値で外す。 */
  install(app: App, target?: Pick<Window, 'addEventListener' | 'removeEventListener'>): () => void
}

/**
 * send の失敗は握りつぶす（ログ送信の失敗がさらに報告を生んで無限に続かないように。ここで投げ直さない）。
 */
export function createErrorReporter(send: SendClientError = defaultSend): ErrorReporter {
  function report(e: ClientError): void {
    const payload: ClientError = {
      kind: e.kind,
      message: clip(e.message, MAX_MESSAGE),
      stack: clip(e.stack, MAX_STACK),
      source: clip(e.source, MAX_MESSAGE),
      code: e.code,
      accountId: e.accountId,
    }
    void send(payload).catch(() => {})
  }

  function install(
    app: App,
    target: Pick<Window, 'addEventListener' | 'removeEventListener'> = window,
  ): () => void {
    const onError = (ev: Event): void => {
      const e = ev as ErrorEvent
      const t = describeThrown(e.error ?? e.message)
      const source = e.filename ? `${e.filename}:${e.lineno}:${e.colno}` : ''
      report({ kind: 'error', message: t.message || e.message, stack: t.stack, source })
    }
    const onRejection = (ev: Event): void => {
      const t = describeThrown((ev as PromiseRejectionEvent).reason)
      report({ kind: 'unhandledrejection', ...t })
    }
    target.addEventListener('error', onError)
    target.addEventListener('unhandledrejection', onRejection)
    const previous = app.config.errorHandler
    app.config.errorHandler = (err, instance, info) => {
      const t = describeThrown(err)
      report({ kind: 'vue', message: t.message, stack: t.stack, source: info })
      if (previous) previous(err, instance, info)
      else console.error(err)
    }
    setBindingErrorReporter((err: AppError) => {
      if (IGNORED_CODES.has(err.code)) return
      report({ kind: 'binding', message: err.message, code: err.code, accountId: err.accountId })
    })
    return () => {
      target.removeEventListener('error', onError)
      target.removeEventListener('unhandledrejection', onRejection)
      app.config.errorHandler = previous
      setBindingErrorReporter(undefined)
    }
  }

  return { report, install }
}
