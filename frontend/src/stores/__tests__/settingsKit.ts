// 設定ストアのテスト共通の土台（バインディング・イベントのモック）。
// bun のモジュールキャッシュはテストファイルをまたぐため、ストアを使うテストは同じ状態（calls・responses など）を共有する。
import { mock } from 'bun:test'
import * as realBindings from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import * as realEvents from '../../lib/events'

const BINDINGS = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'

type Handler = (payload: unknown) => void
export const handlers = new Map<string, Handler>()
export const unsubscribed: string[] = []
export const calls: { name: string; args: unknown[] }[] = []
export const responses: Record<string, unknown> = {}
export const failures: Record<string, unknown> = {}

function binding(name: string) {
  return (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in failures) return Promise.reject(failures[name])
    return Promise.resolve(responses[name])
  }
}

const service = (...names: string[]) => Object.fromEntries(names.map((n) => [n, binding(n)]))

/** ストアを import する前に呼ぶ。 */
export function installMocks(): void {
  mock.module(BINDINGS, () => ({
    AIService: service(
      'GetAIConfig',
      'SetAIConfig',
      'SetAIApiKey',
      'ClearAIApiKey',
      'RecordAIConsent',
    ),
    AppService: service(
      'GetAppInfo',
      'SetUpdateCheckEnabled',
      'CheckForUpdate',
      'SetAutoUpdateEnabled',
      'GetUpdateStatus',
      'RestartToUpdate',
    ),
    ThreadService: service('OpenExternal'),
    MCPService: service('GetMCPStatus', 'SetMCPConfig', 'IssueMCPToken', 'RevokeMCPToken'),
    LogService: service('OpenLogFolder', 'ExportLogs'),
  }))
  mock.module('../../lib/events', () => ({
    ...realEvents,
    onEvent: (name: string, h: Handler) => {
      handlers.set(name, h)
      return () => {
        unsubscribed.push(name)
        handlers.delete(name)
      }
    },
  }))
}

/** 他のテストファイルへモックが漏れないよう、本物へ戻す。 */
export function restoreMocks(): void {
  mock.module(BINDINGS, () => ({ ...realBindings }))
  mock.module('../../lib/events', () => ({ ...realEvents }))
}

export function resetKit(): void {
  handlers.clear()
  unsubscribed.length = 0
  calls.length = 0
  for (const k of Object.keys(failures)) delete failures[k]
  for (const k of Object.keys(responses)) delete responses[k]
}

export const callsOf = (name: string) => calls.filter((c) => c.name === name)
