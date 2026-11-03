// AI 送信先の表示用ロジック（NF §2: OpenAI 互換の Base URL を含め、送信先は常に表示する）。
import { settings as msg } from '../../../lib/messages'

const ANTHROPIC_URL = 'https://api.anthropic.com'
const OPENAI_DEFAULT_URL = 'https://api.openai.com'

export interface AIDestination {
  providerLabel: string
  url: string
  /** Base URL をユーザーが指定している（既定ではない） */
  custom: boolean
}

export function providerLabel(provider: string): string {
  return msg.ai.providerOptions[provider] ?? provider
}

/** 保存済み（または入力中）のプロバイダ・Base URL から、データの送信先を決める。Base URL は openai のみ有効。 */
export function aiDestination(provider: string, baseUrl?: string): AIDestination {
  const label = providerLabel(provider)
  if (provider === 'openai') {
    const custom = (baseUrl ?? '').trim()
    return custom
      ? { providerLabel: label, url: custom, custom: true }
      : { providerLabel: label, url: OPENAI_DEFAULT_URL, custom: false }
  }
  return { providerLabel: label, url: ANTHROPIC_URL, custom: false }
}
