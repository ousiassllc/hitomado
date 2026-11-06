// AI を使う操作（下書き・スレッドの要約と質問）の前提確認（F-08・F-17・F-18）。判定を 1 か所に置き、useAiDraft と useThreadAi が共有する。
// 順序は「設定が未保存→初回の案内」「キー未設定→設定へ誘導」「未同意→同意の画面」。

/** 前提確認に使う AI 設定の項目（AIConfig の一部）。 */
export interface AiPrecheckConfig {
  provider: string
  configured: boolean
  hasApiKey: boolean
  consentedProviders: string[] | null
}

/** AI の設定をまだ保存していない（初めて使う）。保存前の既定のプロバイダではモデルも決まっていないので、先に初回の案内へ進める。 */
export function aiSetupMissing(c: AiPrecheckConfig | undefined): boolean {
  return c?.configured === false
}

/** 設定は保存済みだが、キー（ChatGPT プランはログイン）が無い。 */
export function aiKeyMissing(c: AiPrecheckConfig | undefined): boolean {
  return !aiSetupMissing(c) && c?.hasApiKey !== true
}

/** 現在のプロバイダの送信先に同意していない（設定の未ロード中は同意が必要とは見なさない）。 */
export function aiConsentMissing(c: AiPrecheckConfig | undefined): boolean {
  return c !== undefined && !(c.consentedProviders ?? []).includes(c.provider)
}

/** 前提確認の結果。`ready` なら生成へ進める。 */
export type AiPrecheck = 'setup' | 'key' | 'consent' | 'ready'

/** 次に必要な確認を返す（設定が未保存→キー未設定→未同意の順）。 */
export function aiPrecheck(c: AiPrecheckConfig | undefined): AiPrecheck {
  if (aiSetupMissing(c)) return 'setup'
  if (aiKeyMissing(c)) return 'key'
  if (aiConsentMissing(c)) return 'consent'
  return 'ready'
}
