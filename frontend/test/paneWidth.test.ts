import { describe, expect, test } from 'bun:test'
import {
  clampList,
  clampSidebar,
  LIST,
  MIN_DETAIL,
  MIN_WINDOW_WIDTH,
  SIDEBAR,
} from '../src/lib/paneWidth'

describe('paneWidth', () => {
  test('初期幅は最小ウィンドウ幅でも詳細ペインの最小幅を確保できる', () => {
    expect(MIN_WINDOW_WIDTH - SIDEBAR.initial - LIST.initial).toBeGreaterThanOrEqual(MIN_DETAIL)
  })

  test('範囲内はそのまま、範囲外は丸める', () => {
    expect(clampSidebar(250, 380, 1400)).toBe(250)
    expect(clampSidebar(10, 380, 1400)).toBe(SIDEBAR.min)
    expect(clampSidebar(999, 380, 1400)).toBe(SIDEBAR.max)
    expect(clampList(999, 240, 1400)).toBe(LIST.max)
  })

  test('狭いウィンドウでは右ペインの最小幅を優先して広げすぎない', () => {
    expect(clampList(560, 240, 960)).toBe(960 - 240 - MIN_DETAIL)
    expect(clampSidebar(360, 380, 960)).toBe(960 - 380 - MIN_DETAIL)
  })
})
