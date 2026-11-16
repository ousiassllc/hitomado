import { describe, expect, test } from 'bun:test'
import { revealInline } from './revealInline'

// 幅 300 の窓（scrollLeft から 300px）に、項目 [left, left+width) を入れる。
const scroller = (scrollLeft: number) => ({ scrollLeft, clientWidth: 300 })
const item = (offsetLeft: number, offsetWidth = 100) => ({ offsetLeft, offsetWidth })

describe('revealInline', () => {
  test('窓の右にはみ出している項目は、右端がそろうところまで動かす', () => {
    const s = scroller(0)
    revealInline(s, item(381, 44))
    expect(s.scrollLeft).toBe(125)
  })

  test('窓の左にはみ出している項目は、左端がそろうところまで戻す', () => {
    const s = scroller(200)
    revealInline(s, item(50))
    expect(s.scrollLeft).toBe(50)
  })

  test('すでに見えている項目は動かさない（窓の端にぴったりも含む）', () => {
    const inside = scroller(100)
    revealInline(inside, item(150))
    expect(inside.scrollLeft).toBe(100)
    const edge = scroller(100)
    revealInline(edge, item(300))
    expect(edge.scrollLeft).toBe(100)
  })
})
