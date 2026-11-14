// テスト用: AIConfig の「機能ごとに使う AI」（features）を作る。draft は「AI で下書き」、inboxZero は Inbox Zero に選んだ AI。
import type { AIFeatures } from '../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

export function features(
  draft: { provider: string; model?: string; effort?: string } | string = '',
  inboxZero: { provider: string; model?: string } | string = '',
): AIFeatures {
  const f = (v: { provider: string; model?: string; effort?: string } | string) =>
    typeof v === 'string' ? { provider: v, model: v === '' ? '' : 'm' } : { model: '', ...v }
  return { draft: f(draft), inboxZero: f(inboxZero) }
}
