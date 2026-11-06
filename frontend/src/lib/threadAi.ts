// スレッドの AI 要約・質問（F-17・F-18）の純ロジック。会話を AI へ渡す長さに収める。
import type { ThreadAITurn } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

/** 質問の最大文字数（Go の ai.MaxQuestionRunes と同じ。docs/api §6。#207）。 */
export const MAX_QUESTION_CHARS = 2000

/** AI へ渡す会話の上限（Go の ai.MaxHistoryTurns／MaxHistoryRunes と同じ。最新 10 往復・合計 20,000 文字）。超えたら古い往復から外す。 */
export const MAX_HISTORY_TURNS = 20
export const MAX_HISTORY_CHARS = 20000

/** 画面に出す会話 1 件。 */
export type ThreadAiTurn = { role: 'user' | 'assistant'; text: string }

/** 質問の文字数。Go と同じく前後の空白を除いたコードポイント数で数える。 */
export function questionLength(text: string): number {
  return [...text.trim()].length
}

/**
 * 会話（質問と回答が交互に並ぶ全体）から、AI へ渡す history を作る。新しい往復を優先して最新 10 往復・合計 20,000 文字までに収め、
 * 古い往復から外す。外した往復があれば `dropped` が true（画面に「古いやり取りは文脈から外れています」と出す）。入力は変更しない。
 */
export function trimHistory(turns: readonly ThreadAiTurn[]): {
  history: ThreadAITurn[]
  dropped: boolean
} {
  let keptTurns = 0
  let chars = 0
  for (let i = turns.length - 2; i >= 0 && keptTurns + 2 <= MAX_HISTORY_TURNS; i -= 2) {
    const pair = [...turns[i].text].length + [...turns[i + 1].text].length
    if (chars + pair > MAX_HISTORY_CHARS) break
    chars += pair
    keptTurns += 2
  }
  const history = turns.slice(turns.length - keptTurns).map((t) => ({ role: t.role, text: t.text }))
  return { history, dropped: keptTurns < turns.length }
}
