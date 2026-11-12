import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { effectScope } from 'vue'
import * as realApp from '../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import * as realEvents from '../lib/events'
import * as realCompose from '../stores/compose'

// アプリ終了要求（F-12）: 下書きを保存してから AckBeforeQuit を呼ぶ。保存の成否にかかわらず必ず呼ぶ。

type Handler = (payload: unknown) => void

// モックはプロセス全体で共有される。afterAll で元へ戻せるよう、本物のエクスポートをモックの前に控えておく。
const real = { app: { ...realApp }, events: { ...realEvents }, compose: { ...realCompose } }

const BEFORE_QUIT = 'app:before-quit'
const handlers = new Map<string, Handler>()
const ctl = {
  flush: async (): Promise<boolean> => true,
  ack: async (): Promise<void> => undefined,
}
const log: string[] = []

mock.module('../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice', () => ({
  ...real.app,
  AckBeforeQuit: async () => {
    log.push('ack')
    return ctl.ack()
  },
}))
mock.module('../lib/events', () => ({
  ...real.events,
  EVENTS: { ...real.events.EVENTS, BEFORE_QUIT },
  onEvent: (name: string, h: Handler) => {
    handlers.set(name, h)
    return () => handlers.delete(name)
  },
}))
mock.module('../stores/compose', () => ({
  ...real.compose,
  useComposeStore: () => ({
    flush: () => {
      log.push('flush')
      return ctl.flush()
    },
  }),
}))

// モックを張ったあとに読み込む必要がある（静的 import は巻き上げられ、本物のモジュールを掴んでしまう）。
const { useQuitFlush } = await import('./useQuitFlush')

afterAll(() => {
  mock.module('../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice', () => ({
    ...real.app,
  }))
  mock.module('../lib/events', () => ({ ...real.events }))
  mock.module('../stores/compose', () => ({ ...real.compose }))
})

/** 時間を待たず、次のタスクへ制御を渡す（setTimeout(0) は待ち時間ではなくタスク境界として使う）。 */
function nextTask(): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>()
  setTimeout(resolve, 0)
  return promise
}

/** タスクを回して Promise の連鎖が落ち着くのを待つ。待ちには上限がある（条件待ち）。 */
async function settle(done: () => boolean = () => false): Promise<void> {
  for (let i = 0; i < 50 && !done(); i++) await nextTask()
  for (let i = 0; i < 5; i++) await nextTask() // 余計な呼び出しが無いことの確認
}

/** 未処理の reject を数える。 */
const unhandled: unknown[] = []
const onUnhandled = (e: unknown) => unhandled.push(e)

function mountQuitFlush() {
  const scope = effectScope()
  scope.run(() => useQuitFlush())
  return scope
}

function emitBeforeQuit(): void {
  const h = handlers.get(BEFORE_QUIT)
  if (!h) throw new Error('app:before-quit is not subscribed')
  h(undefined)
}

async function quitAndWaitForAck(): Promise<void> {
  emitBeforeQuit()
  await settle(() => log.includes('ack'))
}

beforeEach(() => {
  handlers.clear()
  log.length = 0
  unhandled.length = 0
  ctl.flush = async () => true
  ctl.ack = async () => undefined
})

describe('useQuitFlush', () => {
  test('保存が成功したら、保存のあとに AckBeforeQuit を 1 回呼ぶ', async () => {
    const scope = mountQuitFlush()
    await quitAndWaitForAck()
    expect(log).toEqual(['flush', 'ack'])
    scope.stop()
  })

  test('保存の完了を待ってから AckBeforeQuit を呼ぶ', async () => {
    const d = Promise.withResolvers<boolean>()
    ctl.flush = () => d.promise
    const scope = mountQuitFlush()
    emitBeforeQuit()
    await settle()
    expect(log).toEqual(['flush']) // 保存が終わるまで Ack しない
    d.resolve(true)
    await settle(() => log.includes('ack'))
    expect(log).toEqual(['flush', 'ack'])
    scope.stop()
  })

  test('保存が失敗（false）でも AckBeforeQuit を 1 回呼ぶ', async () => {
    ctl.flush = async () => false
    const scope = mountQuitFlush()
    await quitAndWaitForAck()
    expect(log).toEqual(['flush', 'ack'])
    scope.stop()
  })

  test('保存が reject しても AckBeforeQuit を 1 回呼ぶ（未処理の reject も出さない）', async () => {
    process.on('unhandledRejection', onUnhandled)
    try {
      ctl.flush = async () => {
        throw new Error('save failed')
      }
      const scope = mountQuitFlush()
      await quitAndWaitForAck()
      expect(log).toEqual(['flush', 'ack'])
      expect(unhandled).toEqual([])
      scope.stop()
    } finally {
      process.off('unhandledRejection', onUnhandled)
    }
  })

  test('AckBeforeQuit が reject しても未処理の reject を出さない', async () => {
    process.on('unhandledRejection', onUnhandled)
    try {
      ctl.ack = async () => {
        throw new Error('ack failed')
      }
      const scope = mountQuitFlush()
      await quitAndWaitForAck()
      expect(log).toEqual(['flush', 'ack'])
      expect(unhandled).toEqual([])
      scope.stop()
    } finally {
      process.off('unhandledRejection', onUnhandled)
    }
  })

  test('スコープ（コンポーネント）の破棄で購読を解除する', () => {
    const scope = mountQuitFlush()
    expect(handlers.has(BEFORE_QUIT)).toBe(true)
    scope.stop()
    expect(handlers.has(BEFORE_QUIT)).toBe(false)
  })
})
