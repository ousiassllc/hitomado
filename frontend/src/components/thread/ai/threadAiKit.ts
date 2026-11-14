// スレッドの AI（threadAiStore・ThreadPane の AI パネル）のテスト用ハーネス。AIService のバインディングをモックする。
// スレッド側のモック（threadTestKit）と合わせて使う。ストアを import する前に installAiMocks() を呼ぶ。
import { mock } from 'bun:test'
import * as realBindings from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type { ThreadAIResult } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { installMocks, resetState, restoreMocks } from '../threadTestKit'

// AIService は index（バインディングの束）から import されるので、index をモックする（葉のモジュールは先に評価されていると効かない）。
const APP = '../../../../bindings/github.com/ousiassllc/hitomado/internal/app'

export type AiCall = { name: string; args: unknown[] }
export const aiCalls: AiCall[] = []
/** 応答。関数ならその戻り値（Promise 可）、値ならそのまま返す。failures に名前があれば reject する。 */
export const aiCtl: { responses: Record<string, unknown>; failures: Record<string, unknown> } = {
  responses: {},
  failures: {},
}

function binding(name: string) {
  return async (...args: unknown[]) => {
    aiCalls.push({ name, args })
    if (name in aiCtl.failures) throw aiCtl.failures[name]
    const r = aiCtl.responses[name]
    return typeof r === 'function' ? await (r as (...a: unknown[]) => unknown)(...args) : r
  }
}

export const aiCallsOf = (name: string) => aiCalls.filter((c) => c.name === name)

export function result(text: string, included = 1, total = 1): ThreadAIResult {
  return { text, includedCount: included, totalCount: total }
}

/** 設定済み・同意済みの AI 設定（前提確認を通る）。 */
export const READY_CONFIG = {
  features: {
    draft: { provider: 'anthropic', model: 'claude-x' },
    inboxZero: { provider: '', model: '' },
  },
  apiKeyProviders: ['anthropic'],
  consentedProviders: ['anthropic'],
  chatgptSignedIn: false,
  chatgptAccounts: [],
}

export function installAiMocks(): void {
  installMocks()
  mock.module(APP, () => ({
    ...realBindings,
    AIService: {
      ...realBindings.AIService,
      SummarizeThread: binding('SummarizeThread'),
      AskAboutThread: binding('AskAboutThread'),
      CancelGenerate: binding('CancelGenerate'),
      GetAIConfig: binding('GetAIConfig'),
      RecordAIConsent: binding('RecordAIConsent'),
    },
  }))
}

export function restoreAiMocks(): void {
  restoreMocks()
  mock.module(APP, () => ({ ...realBindings }))
}

/** 各テストの前に呼ぶ。 */
export function resetAiState(): void {
  resetState()
  aiCalls.length = 0
  aiCtl.failures = {}
  aiCtl.responses = {
    SummarizeThread: result('- 要約'),
    AskAboutThread: result('回答'),
    CancelGenerate: undefined,
    GetAIConfig: READY_CONFIG,
    RecordAIConsent: READY_CONFIG,
  }
}
