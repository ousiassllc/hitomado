// フロントのエラー（未処理の例外・Promise の reject・Vue の描画エラー・バインディング呼び出しの失敗）を Go 側のログへ送る。
// 例外のメッセージは件名・本文・宛先の断片（テンプレート埋め込みの例外など）を含みうるので送らない。
// message には種類のコード（TypeError・unknown・render-error:コンポーネント名 など）だけを入れ、stack は「関数名とファイル:行:列」のフレームだけにする。
// binding はエラーコード（enum）とアカウント ID だけを送る。Go 側のログ出力（logging.Redact）の伏せ処理は 2 枚目の防御として残る。
import type { App, ComponentPublicInstance } from 'vue'
import { LogService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type { ClientError } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { type AppError, isAppErrorLike, setBindingErrorReporter } from './api'

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

/** 種類のコードとして送ってよいエラークラス名（ここにない名前は message 由来の文字列を含みうるので送らない）。 */
const ERROR_CLASSES: ReadonlySet<string> = new Set([
  'Error',
  'TypeError',
  'RangeError',
  'ReferenceError',
  'SyntaxError',
  'EvalError',
  'URIError',
  'AggregateError',
  'DOMException',
])

const MAX_FRAMES = 50
const SAFE_CODE = /^[A-Z_]{1,40}$/
const SAFE_IDENT = /^[\w$.<>[\] -]{1,100}$/
// 例: "    at fn (file:///app.js:3:7)" / "    at file:///app.js:3:7"（V8）
const V8_FRAME = /^\s+at (?:(.+?) \()?([^\s()]+?):(\d+):(\d+)\)?$/
// 例: "fn@file:///app.js:3:7"（JavaScriptCore・Firefox。WebKitGTK はこちら）
const GECKO_FRAME = /^([\w$.<>/]*)@(\S+?):(\d+):(\d+)$/

/** 例外値から message に載せる種類のコードを決める。メッセージ本文は見ない。 */
export function errorKindCode(v: unknown): string {
  if (v instanceof Error) return ERROR_CLASSES.has(v.name) ? v.name : 'Error'
  return 'unknown'
}

/** スタックから「関数名とファイル:行:列」のフレームだけを取り出す。メッセージ本文（先頭行など）は捨てる。 */
export function sanitizeStack(v: unknown): string {
  if (!(v instanceof Error) || !v.stack) return ''
  let stack = v.stack
  const header = `${v.name}: ${v.message}`
  if (stack.startsWith(header)) stack = stack.slice(header.length)
  const frames: string[] = []
  for (const line of stack.split('\n')) {
    const m = V8_FRAME.exec(line) ?? GECKO_FRAME.exec(line)
    if (!m) continue
    const fn = m[1] && SAFE_IDENT.test(m[1]) ? m[1] : ''
    frames.push(`${fn}@${m[2]}:${m[3]}:${m[4]}`)
    if (frames.length >= MAX_FRAMES) break
  }
  return frames.join('\n')
}

/** 例外値（Error・文字列・その他）から送る message（種類のコード）と stack（位置だけ）を取り出す。 */
export function describeThrown(v: unknown): { message: string; stack: string } {
  return { message: errorKindCode(v), stack: sanitizeStack(v) }
}

/** Vue の描画エラーで送るコンポーネント名。名前が無い・安全でない文字を含むときは anonymous。 */
function componentName(instance: ComponentPublicInstance | null): string {
  const o = instance?.$options as { name?: unknown; __name?: unknown } | undefined
  const n = o?.name ?? o?.__name
  return typeof n === 'string' && SAFE_IDENT.test(n) ? n : 'anonymous'
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
    try {
      void send(payload).catch(() => {})
    } catch {
      // send が同期的に投げても握りつぶす（ハンドラの中で再び例外を起こさない）。
    }
  }

  function install(
    app: App,
    target: Pick<Window, 'addEventListener' | 'removeEventListener'> = window,
  ): () => void {
    const onError = (ev: Event): void => {
      const e = ev as ErrorEvent
      const t = describeThrown(e.error)
      const source = e.filename ? `${e.filename}:${e.lineno}:${e.colno}` : ''
      report({ kind: 'error', ...t, source })
    }
    const onRejection = (ev: Event): void => {
      const reason = (ev as PromiseRejectionEvent).reason
      // call() が投げ直した AppError は、その場で binding として報告済み。二重に報告しない。
      if (isAppErrorLike(reason)) return
      const t = describeThrown(reason)
      report({ kind: 'unhandledrejection', ...t })
    }
    target.addEventListener('error', onError)
    target.addEventListener('unhandledrejection', onRejection)
    const previous = app.config.errorHandler
    app.config.errorHandler = (err, instance, info) => {
      const t = describeThrown(err)
      // 描画エラーはメッセージ・props を送らず、コンポーネント名だけにする。
      report({
        kind: 'vue',
        message: `render-error:${componentName(instance)}`,
        stack: t.stack,
        source: info,
      })
      if (previous) previous(err, instance, info)
      else console.error(err)
    }
    setBindingErrorReporter((err: AppError) => {
      if (IGNORED_CODES.has(err.code)) return
      // AppError.message は Go 側の文面（外部 API の応答を含みうる）なので送らない。コードで足りる。
      const code = SAFE_CODE.test(err.code) ? err.code : 'INTERNAL'
      report({ kind: 'binding', message: code, code, accountId: err.accountId })
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
