import { describe, expect, test } from 'bun:test'
import { formatDate, formatDateTime, formatSize } from '../src/lib/format'

const now = new Date(2026, 10, 2, 15, 30) // 2026-11-02 15:30 ローカル

describe('formatDate', () => {
  test('今日は時刻', () => {
    expect(formatDate(new Date(2026, 10, 2, 9, 5).getTime(), now)).toBe('09:05')
    expect(formatDate(new Date(2026, 10, 2, 0, 0).getTime(), now)).toBe('00:00')
  })

  test('今年は月日', () => {
    expect(formatDate(new Date(2026, 10, 1, 23, 59).getTime(), now)).toBe('11月1日')
    expect(formatDate(new Date(2026, 0, 31, 8, 0).getTime(), now)).toBe('1月31日')
  })

  test('それ以外は年月日', () => {
    expect(formatDate(new Date(2025, 11, 31, 8, 0).getTime(), now)).toBe('2025年12月31日')
  })

  test('同じ日付でも年が違えば年月日', () => {
    expect(formatDate(new Date(2025, 10, 2, 8, 0).getTime(), now)).toBe('2025年11月2日')
  })
})

test('formatDateTime', () => {
  expect(formatDateTime(new Date(2026, 2, 4, 7, 8).getTime())).toBe('2026年3月4日 07:08')
})

describe('formatSize', () => {
  test('各単位', () => {
    expect(formatSize(0)).toBe('0 B')
    expect(formatSize(1023)).toBe('1023 B')
    expect(formatSize(1024)).toBe('1.0 KB')
    expect(formatSize(1536)).toBe('1.5 KB')
    expect(formatSize(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(formatSize(1024 ** 3 * 2)).toBe('2.0 GB')
  })

  test('不正値は 0 B', () => {
    expect(formatSize(-1)).toBe('0 B')
    expect(formatSize(Number.NaN)).toBe('0 B')
  })
})
