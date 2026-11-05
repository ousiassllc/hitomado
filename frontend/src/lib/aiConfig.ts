import type {
  AIConfig,
  AIModelList,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { list } from './api'

/** モデル一覧の保存キー。OpenAI 互換は Base URL ごとに別の一覧になる（Base URL が空の既定の送信先は、プロバイダ名だけ）。 */
export function aiModelsKey(provider: string, baseUrl = ''): string {
  const url = baseUrl.trim()
  return provider === 'openai' && url !== '' ? `${provider}|${url}` : provider
}

/** Go は consentedProviders・apiKeyProviders・chatgptAccounts を nil にし得るので、画面側は常に配列として扱う。 */
export function normalizeAIConfig(c: AIConfig): AIConfig {
  return {
    ...c,
    consentedProviders: list(c.consentedProviders),
    apiKeyProviders: list(c.apiKeyProviders),
    chatgptAccounts: list(c.chatgptAccounts),
  }
}

/** OpenAI 互換と ChatGPT プランの共通のエフォートの段階（一覧がモデルごとの段階を返さないため。Go の ai.EffortScale と同じ。弱い順）。 */
export const COMMON_EFFORTS = ['low', 'medium', 'high']

/**
 * 画面に出すエフォートの段階（弱い順。空なら選択自体を出さない。#111）。
 * anthropic は選んだモデルの `AIModel.efforts` だけ（対応しないモデル・一覧が取れず手入力のモデルは空）。openai／chatgpt は共通の 3 段階。
 */
export function effortChoices(
  provider: string,
  model: string,
  models: AIModelList | undefined,
): string[] {
  if (provider !== 'anthropic') return COMMON_EFFORTS
  return list(list(models?.models).find((m) => m.id === model.trim())?.efforts)
}
