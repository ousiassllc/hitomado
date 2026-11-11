// AI のプロバイダの一覧（#315）。表示順どおり。`kind` は設定ページの種類:
//   draft = 「AI で下書き」に使えるプロバイダ（モデル・同意・API キーまたはログイン。AISettings）、
//   key   = API キーだけを登録するプロバイダ（文章を書かないモデル。AITypeSafePage）。
import type { AIConfig } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

export type AIProviderId = 'chatgpt' | 'anthropic' | 'openai' | 'typesafe'

export interface AIProviderEntry {
  id: AIProviderId
  kind: 'draft' | 'key'
}

export const AI_PROVIDERS: readonly AIProviderEntry[] = [
  { id: 'chatgpt', kind: 'draft' },
  { id: 'anthropic', kind: 'draft' },
  { id: 'openai', kind: 'draft' },
  { id: 'typesafe', kind: 'key' },
]

export function providerKind(id: string): AIProviderEntry['kind'] | undefined {
  return AI_PROVIDERS.find((p) => p.id === id)?.kind
}

/** そのプロバイダの資格情報を持っているか（ChatGPT はログイン、ほかは API キー）。 */
export function hasProviderCredential(id: string, cfg: AIConfig | undefined): boolean {
  if (id === 'chatgpt') return cfg?.chatgptSignedIn ?? false
  return cfg?.apiKeyProviders?.includes(id) ?? false
}
