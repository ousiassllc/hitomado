import type { AIConfig } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { hasProviderCredential } from '../../../lib/aiConfig'
import type { AiPrecheckConfig } from '../../../lib/aiPrecheck'

/**
 * 「AI で下書き」（下書き・要約・質問）の前提確認に渡す形（#376）。機能ごとに選んだプロバイダの資格情報・同意を見る。
 * 選んでいない（provider が空）ときは未設定（configured=false）で、初めて使うときの案内を出す。
 */
export function draftPrecheckConfig(c: AIConfig | undefined): AiPrecheckConfig | undefined {
  if (c === undefined) return undefined
  const { provider } = c.features.draft
  return {
    provider,
    configured: provider !== '',
    hasApiKey: hasProviderCredential(provider, c),
    consentedProviders: c.consentedProviders,
  }
}
