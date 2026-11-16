// テスト用: AIConfig の「機能ごとに使う AI」（features）を作る。draft は「AI で下書き」、inboxZero は Inbox Zero、
// threadAi は「要約・質問」に選んだ AI（省略は未選択。「AI で下書き」からは引き継がない）。
import type { AIFeatures } from '../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

type Pick = { provider: string; model?: string; effort?: string } | string

export function features(draft: Pick = '', inboxZero: Pick = '', threadAi: Pick = ''): AIFeatures {
  const f = (v: Pick) =>
    typeof v === 'string' ? { provider: v, model: v === '' ? '' : 'm' } : { model: '', ...v }
  return { draft: f(draft), threadAi: f(threadAi), inboxZero: f(inboxZero) }
}
