import type { AIConfig } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
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
