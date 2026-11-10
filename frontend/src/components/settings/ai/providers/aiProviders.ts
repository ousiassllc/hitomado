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

// プロバイダの一覧（AIProviderList）のカードのグリッドの列数（#315）。画面ではなく一覧の幅（設定ダイアログの本文の幅）で決める。
// デスクトップ（設定ダイアログ lg の本文 806px）は 4 列、Android の幅（390px の画面で一覧 324px）は 2 列、
// 一覧が 280px 未満（320px の画面で 254px など）は 1 列。カードの幅が 1 列あたり約 130px を下回らないように境目を置く。
/** この幅（px）以上は 4 列。 */
export const GRID_FOUR_MIN_WIDTH = 600
/** この幅（px）以上は 2 列、未満は 1 列。 */
export const GRID_TWO_MIN_WIDTH = 280

export type ProviderGridColumns = 1 | 2 | 4

/** 一覧の幅（px。測れていない 0 以下も含む）から列数を決める。 */
export function providerGridColumns(width: number): ProviderGridColumns {
  if (width >= GRID_FOUR_MIN_WIDTH) return 4
  if (width >= GRID_TWO_MIN_WIDTH) return 2
  return 1
}
