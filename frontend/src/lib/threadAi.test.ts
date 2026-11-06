import { describe, expect, test } from 'bun:test'
import {
  MAX_HISTORY_CHARS,
  MAX_HISTORY_TURNS,
  MAX_QUESTION_CHARS,
  questionLength,
  type ThreadAiTurn,
  trimHistory,
} from './threadAi'

function convo(pairs: number, size = 1): ThreadAiTurn[] {
  return Array.from({ length: pairs }, (_, i) => [
    { role: 'user' as const, text: `q${i + 1}`.padEnd(size, 'q') },
    { role: 'assistant' as const, text: `a${i + 1}`.padEnd(size, 'a') },
  ]).flat()
}

describe('trimHistory', () => {
  test('空の会話は空の history で、外していない', () => {
    expect(trimHistory([])).toEqual({ history: [], dropped: false })
  })

  test('上限内ならそのまま渡す（入力は変更しない）', () => {
    const turns = convo(10)
    const r = trimHistory(turns)
    expect(r.history).toEqual(turns)
    expect(r.dropped).toBe(false)
    expect(r.history).not.toBe(turns)
  })

  test('10 往復を超えたら、古い往復から外して最新の 10 往復にする', () => {
    const r = trimHistory(convo(12))
    expect(r.history).toHaveLength(MAX_HISTORY_TURNS)
    expect(r.history[0].text).toBe('q3')
    expect(r.history.at(-1)?.text).toBe('a12')
    expect(r.dropped).toBe(true)
  })

  test('合計 20,000 文字を超えたら、往復の単位で古い方から外す', () => {
    // 1 往復 = 4,000 文字 → 5 往復（20,000 文字ちょうど）まで入る
    const r = trimHistory(convo(7, 2000))
    expect(r.history).toHaveLength(10)
    expect(r.history[0].text.startsWith('q3')).toBe(true)
    const total = r.history.reduce((n, t) => n + [...t.text].length, 0)
    expect(total).toBeLessThanOrEqual(MAX_HISTORY_CHARS)
    expect(r.dropped).toBe(true)
  })

  test('文字数はコードポイントで数える（Go の rune 数と同じ）', () => {
    const turns: ThreadAiTurn[] = [
      { role: 'user', text: '😀'.repeat(10000) },
      { role: 'assistant', text: '😀'.repeat(10000) },
    ]
    expect(trimHistory(turns).history).toHaveLength(2)
    turns[1].text += '😀'
    expect(trimHistory(turns).history).toHaveLength(0)
  })
})

describe('questionLength', () => {
  test('前後の空白を除いたコードポイント数（Go の MaxQuestionRunes と同じ 2,000）', () => {
    expect(MAX_QUESTION_CHARS).toBe(2000)
    expect(questionLength('  ')).toBe(0)
    expect(questionLength(' あ😀 ')).toBe(2)
  })
})
