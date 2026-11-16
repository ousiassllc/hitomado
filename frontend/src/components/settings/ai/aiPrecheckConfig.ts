import type { AIConfig } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { hasProviderCredential } from '../../../lib/aiConfig'
import type { AiPrecheckConfig } from '../../../lib/aiPrecheck'

/**
 * 機能（draft／threadAi）の前提確認に渡す形（#376）。機能ごとに選んだプロバイダの資格情報・同意を見る。
 * 選んでいない（provider が空）ときは未設定（configured=false）で、初めて使うときの案内を出す。
 */
function precheckConfig(
  c: AIConfig | undefined,
  feature: 'draft' | 'threadAi',
): AiPrecheckConfig | undefined {
  if (c === undefined) return undefined
  const { provider } = c.features[feature]
  return {
    provider,
    configured: provider !== '',
    hasApiKey: hasProviderCredential(provider, c),
    consentedProviders: c.consentedProviders,
  }
}

/** 「AI で下書き」（返信・新規作成）の前提確認。 */
export const draftPrecheckConfig = (c: AIConfig | undefined) => precheckConfig(c, 'draft')

/** 「要約・質問」（スレッドの AI）の前提確認。「AI で下書き」の選択は見ない。 */
export const threadAiPrecheckConfig = (c: AIConfig | undefined) => precheckConfig(c, 'threadAi')
