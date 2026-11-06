import { describe, expect, test } from 'bun:test'
import { createApp } from 'vue'
import type { ClientError } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call } from '../api'
import { createErrorReporter, describeThrown, sanitizeStack } from './index'

class FakeWindow {
  private handlers = new Map<string, Set<(e: Event) => void>>()
  addEventListener(type: string, fn: (e: Event) => void): void {
    this.handlers.set(type, (this.handlers.get(type) ?? new Set()).add(fn))
  }
  removeEventListener(type: string, fn: (e: Event) => void): void {
    this.handlers.get(type)?.delete(fn)
  }
  dispatch(type: string, ev: object): void {
    for (const fn of this.handlers.get(type) ?? []) fn(ev as Event)
  }
}

function setup() {
  const sent: ClientError[] = []
  const reporter = createErrorReporter(async (e) => {
    sent.push(e)
  })
  const app = createApp({ render: () => null })
  app.config.errorHandler = () => {} // 既定のハンドラは console.error に出すので、無害な元ハンドラに差し替える
  const win = new FakeWindow()
  const uninstall = reporter.install(app, win as unknown as Window)
  return { sent, reporter, app, win, uninstall }
}

describe('errorReporter', () => {
  test('未処理の例外（error イベント）を発生元つきで送る', () => {
    const { sent, win } = setup()
    const err = new Error('boom')
    win.dispatch('error', { error: err, message: 'boom', filename: 'app.js', lineno: 3, colno: 7 })
    expect(sent).toHaveLength(1)
    expect(sent[0]).toMatchObject({ kind: 'error', message: 'Error', source: 'app.js:3:7' })
    expect(sent[0]?.stack).not.toContain('boom')
  })

  test('Promise の reject を送る（Error 以外の値は中身を文字列化せず型だけ）', () => {
    const { sent, win } = setup()
    win.dispatch('unhandledrejection', { reason: new Error('rejected') })
    win.dispatch('unhandledrejection', { reason: { to: 'alice@example.com', body: 'secret' } })
    expect(sent.map((s) => s.kind)).toEqual(['unhandledrejection', 'unhandledrejection'])
    expect(sent[0]?.message).toBe('Error')
    expect(sent[1]?.message).toBe('unknown')
    expect(JSON.stringify(sent[1])).not.toContain('alice@example.com')
  })

  test('call() が投げ直した AppError の未処理 reject は二重に報告しない', () => {
    const { sent, win } = setup()
    win.dispatch('unhandledrejection', {
      reason: { code: 'GMAIL_ERROR', message: 'm', retryable: false },
    })
    expect(sent).toHaveLength(0)
  })

  test('send が同期的に投げても例外を外へ出さない', () => {
    const reporter = createErrorReporter(() => {
      throw new Error('sync failure')
    })
    expect(() => reporter.report({ kind: 'error', message: 'x' })).not.toThrow()
  })

  test('Vue の描画エラーを送り、元のハンドラも呼ぶ', () => {
    const sent: ClientError[] = []
    const reporter = createErrorReporter(async (e) => {
      sent.push(e)
    })
    const app = createApp({ render: () => null })
    const seen: unknown[] = []
    app.config.errorHandler = (e) => seen.push(e)
    reporter.install(app, new FakeWindow() as unknown as Window)
    const err = new TypeError('render failed')
    app.config.errorHandler?.(err, null, 'render function')
    expect(sent[0]).toMatchObject({
      kind: 'vue',
      message: 'render-error:anonymous',
      source: 'render function',
    })
    expect(seen).toEqual([err])
  })

  test('Wails 呼び出しの失敗を コードとアカウント ID つきで送る。取り消しは送らない', async () => {
    const { sent } = setup()
    const rejected = (code: string, accountId?: string) =>
      Promise.reject({ code, message: 'm', retryable: false, accountId })
    await expect(call(rejected('GMAIL_ERROR', '01HX'))).rejects.toMatchObject({
      code: 'GMAIL_ERROR',
    })
    await expect(call(rejected('CANCELLED'))).rejects.toMatchObject({ code: 'CANCELLED' })
    await expect(call(rejected('OAUTH_CANCELLED'))).rejects.toMatchObject({
      code: 'OAUTH_CANCELLED',
    })
    expect(sent).toHaveLength(1)
    expect(sent[0]).toMatchObject({
      kind: 'binding',
      code: 'GMAIL_ERROR',
      accountId: '01HX',
      message: 'GMAIL_ERROR',
    })
  })

  test('送る項目は決まったものだけ（本文・件名・宛先の項目を持たない）', () => {
    const { sent, reporter } = setup()
    reporter.report({ kind: 'error', message: 'x', stack: '', source: '', code: '', accountId: '' })
    expect(Object.keys(sent[0] ?? {}).sort()).toEqual(
      ['accountId', 'code', 'kind', 'message', 'source', 'stack'].sort(),
    )
  })

  test('長いメッセージ・スタックは切る', () => {
    const { sent, reporter } = setup()
    reporter.report({
      kind: 'error',
      message: 'a'.repeat(5000),
      stack: 'b'.repeat(20000),
      source: '',
    })
    expect(sent[0]?.message.length).toBe(1000)
    expect(sent[0]?.stack?.length).toBe(8000)
  })

  test('送信の失敗は握りつぶす（再び報告しない）', async () => {
    let calls = 0
    const reporter = createErrorReporter(async () => {
      calls++
      throw new Error('log service down')
    })
    reporter.report({ kind: 'error', message: 'x' })
    await Promise.resolve()
    await Promise.resolve()
    expect(calls).toBe(1)
  })

  test('取り外すと以後は送らない', async () => {
    const { sent, win, uninstall } = setup()
    uninstall()
    win.dispatch('error', { error: new Error('after'), message: 'after' })
    await expect(call(Promise.reject({ code: 'INTERNAL', message: 'm' }))).rejects.toBeDefined()
    expect(sent).toHaveLength(0)
  })

  test('describeThrown', () => {
    expect(describeThrown('plain')).toEqual({ message: 'unknown', stack: '' })
    expect(describeThrown(42).message).toBe('unknown')
    expect(describeThrown(new RangeError('x')).message).toBe('RangeError')
  })

  describe('例外メッセージを送らない', () => {
    // 件名・本文・宛先の断片が例外メッセージに混ざった状況の模擬（実在のメールではない）。
    const LEAK = '件名: 来週の打ち合わせ / 本文: 見積もりは下記のとおりです / alice@example.com'
    const leaky = () => new TypeError(`Cannot read ${LEAK}`)

    function expectNoLeak(sent: ClientError[]): void {
      expect(sent).toHaveLength(1)
      const json = JSON.stringify(sent[0])
      for (const frag of ['来週の打ち合わせ', '見積もり', 'alice@example.com', 'Cannot read']) {
        expect(json).not.toContain(frag)
      }
    }

    test('error イベント', () => {
      const { sent, win } = setup()
      const err = leaky()
      win.dispatch('error', {
        error: err,
        message: err.message,
        filename: 'app.js',
        lineno: 1,
        colno: 2,
      })
      expectNoLeak(sent)
      expect(sent[0]?.message).toBe('TypeError')
    })

    test('error イベントの Error 無し（message だけ）', () => {
      const { sent, win } = setup()
      win.dispatch('error', { message: LEAK })
      expectNoLeak(sent)
      expect(sent[0]?.message).toBe('unknown')
    })

    test('unhandledrejection（Error・文字列・オブジェクト）', () => {
      const { sent, win } = setup()
      win.dispatch('unhandledrejection', { reason: leaky() })
      win.dispatch('unhandledrejection', { reason: LEAK })
      win.dispatch('unhandledrejection', { reason: { message: LEAK } })
      expect(sent.map((s) => s.message)).toEqual(['TypeError', 'unknown', 'unknown'])
      expect(JSON.stringify(sent)).not.toContain('alice@example.com')
      expect(JSON.stringify(sent)).not.toContain('来週の打ち合わせ')
    })

    test('Vue の描画エラーはコンポーネント名だけ', () => {
      const { sent, app } = setup()
      const instance = { $options: { name: 'MailComposer' } }
      app.config.errorHandler?.(leaky(), instance as never, 'render function')
      expectNoLeak(sent)
      expect(sent[0]).toMatchObject({ kind: 'vue', message: 'render-error:MailComposer' })
    })

    test('Vue のコンポーネント名に任意の文字列は載せない', () => {
      const { sent, app } = setup()
      app.config.errorHandler?.(leaky(), { $options: { name: LEAK } } as never, 'setup function')
      expectNoLeak(sent)
      expect(sent[0]?.message).toBe('render-error:anonymous')
    })

    test('独自のエラー名（name にメッセージ由来の文字列）は Error にまとめる', () => {
      const { sent, win } = setup()
      const err = leaky()
      err.name = LEAK
      win.dispatch('error', { error: err, message: err.message })
      expectNoLeak(sent)
      expect(sent[0]?.message).toBe('Error')
    })

    test('binding はメッセージを送らずコードだけ（コードも列挙形式のみ）', async () => {
      const { sent } = setup()
      await expect(
        call(Promise.reject({ code: 'GMAIL_ERROR', message: LEAK, retryable: false })),
      ).rejects.toBeDefined()
      await expect(
        call(Promise.reject({ code: LEAK, message: LEAK, retryable: false })),
      ).rejects.toBeDefined()
      expect(JSON.stringify(sent)).not.toContain('alice@example.com')
      expect(JSON.stringify(sent)).not.toContain('来週の打ち合わせ')
      expect(sent.map((s) => s.code)).toEqual(['GMAIL_ERROR', 'INTERNAL'])
    })
  })

  describe('sanitizeStack', () => {
    function withStack(stack: string, message = 'x'): Error {
      const e = new Error(message)
      e.stack = stack
      return e
    }

    test('V8 形式: 先頭のメッセージ行（複数行でも）を捨てて位置だけ残す', () => {
      const message = '件名: 秘密\n本文: 内容 alice@example.com'
      const e = withStack(
        `Error: ${message}\n    at render (file:///app/main.js:10:5)\n    at file:///app/a.js:1:2`,
        message,
      )
      expect(sanitizeStack(e)).toBe('render@file:///app/main.js:10:5\n@file:///app/a.js:1:2')
    })

    test('JavaScriptCore 形式', () => {
      const e = withStack(
        'render@http://localhost/assets/main.js:10:5\nglobal code@http://x/a.js:1:2',
      )
      expect(sanitizeStack(e)).toBe('render@http://localhost/assets/main.js:10:5')
    })

    test('フレームでない行・Error 以外は捨てる', () => {
      expect(sanitizeStack(withStack('Error: alice@example.com\nsome free text'))).toBe('')
      expect(sanitizeStack('Error: alice@example.com')).toBe('')
    })

    test('フレーム数を切る', () => {
      const frames = Array.from({ length: 80 }, (_, i) => `    at f${i} (a.js:${i}:1)`)
      expect(sanitizeStack(withStack(['Error: x', ...frames].join('\n'))).split('\n')).toHaveLength(
        50,
      )
    })
  })
})
