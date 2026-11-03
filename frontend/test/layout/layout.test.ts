import { describe, expect, test } from 'bun:test'
import {
  BREAKPOINT_MEDIUM,
  BREAKPOINT_WIDE,
  layoutModeFor,
  MEDIUM_QUERY,
  WIDE_QUERY,
} from '../../src/lib/layout'
import { MIN_WINDOW_WIDTH } from '../../src/lib/paneWidth'

describe('layoutModeFor', () => {
  test('タッチ UI でない OS（Windows）は幅に関わらず常に 3 ペイン', () => {
    expect(layoutModeFor(false, true, true)).toBe('wide')
    expect(layoutModeFor(false, false, true)).toBe('wide')
    expect(layoutModeFor(false, false, false)).toBe('wide')
  })

  test('タッチ UI: 広い→3 ペイン、中くらい→2 ペイン、狭い→1 画面ずつ', () => {
    expect(layoutModeFor(true, true, true)).toBe('wide')
    expect(layoutModeFor(true, false, true)).toBe('medium')
    expect(layoutModeFor(true, false, false)).toBe('stack')
  })

  test('ブレイクポイントは 1 か所の定数。3 ペインはデスクトップの最小幅（960px）以上', () => {
    expect(BREAKPOINT_WIDE).toBe(MIN_WINDOW_WIDTH)
    expect(BREAKPOINT_MEDIUM).toBeLessThan(BREAKPOINT_WIDE)
    expect(WIDE_QUERY).toBe(`(min-width: ${BREAKPOINT_WIDE}px)`)
    expect(MEDIUM_QUERY).toBe(`(min-width: ${BREAKPOINT_MEDIUM}px)`)
  })
})
