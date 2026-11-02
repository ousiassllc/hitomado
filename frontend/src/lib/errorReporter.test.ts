import { describe, expect, test } from 'bun:test'
import { createApp } from 'vue'
import type { ClientError } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call } from './api'
import { createErrorReporter, describeThrown } from './errorReporter'

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
    expect(sent[0]).toMatchObject({ kind: 'error', message: 'boom', source: 'app.js:3:7' })
    expect(sent[0]?.stack).toContain('boom')
  })

  test('Promise の reject を送る（Error 以外の値は中身を文字列化せず型だけ）', () => {
    const { sent, win } = setup()
    win.dispatch('unhandledrejection', { reason: new Error('rejected') })
    win.dispatch('unhandledrejection', { reason: { to: 'alice@example.com', body: 'secret' } })
    expect(sent.map((s) => s.kind)).toEqual(['unhandledrejection', 'unhandledrejection'])
    expect(sent[0]?.message).toBe('rejected')
    expect(sent[1]?.message).toBe('non-error value (object)')
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
    const err = new Error('render failed')
    app.config.errorHandler?.(err, null, 'render function')
    expect(sent[0]).toMatchObject({
      kind: 'vue',
      message: 'render failed',
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
      message: 'm',
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
    expect(describeThrown('plain')).toEqual({ message: 'plain', stack: '' })
    expect(describeThrown(42).message).toBe('non-error value (number)')
  })
})
