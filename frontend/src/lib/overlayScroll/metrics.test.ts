import { describe, expect, test } from 'bun:test'
import { CORNER, hasMore, INSET, MIN_THUMB, overflows, scrollDeltaFor, thumbFor } from './metrics'

describe('thumbFor', () => {
  test('はみ出していなければ出さない（1px 未満の丸め誤差も）', () => {
    expect(thumbFor({ scroll: 0, size: 500, client: 500 }, 0)).toBeNull()
    expect(thumbFor({ scroll: 0, size: 500.5, client: 500 }, 0)).toBeNull()
  })

  test('長さは見えている割合。端から 2px 内側に置く', () => {
    const t = thumbFor({ scroll: 0, size: 1000, client: 500 }, 0)
    expect(t?.length).toBeCloseTo((500 - INSET * 2) / 2)
    expect(t?.offset).toBe(INSET)
  })

  test('いちばん下では、つまみの下端が端から 2px 内側', () => {
    const t = thumbFor({ scroll: 500, size: 1000, client: 500 }, 0)
    expect((t?.offset ?? 0) + (t?.length ?? 0)).toBeCloseTo(500 - INSET)
  })

  test('長さは最低 32px', () => {
    const t = thumbFor({ scroll: 0, size: 100_000, client: 500 }, 0)
    expect(t?.length).toBe(MIN_THUMB)
  })

  test('反対の軸のバーがあるときは隅を空ける', () => {
    const t = thumbFor({ scroll: 500, size: 1000, client: 500 }, CORNER)
    expect((t?.offset ?? 0) + (t?.length ?? 0)).toBeCloseTo(500 - INSET - CORNER)
  })

  test('領域が小さすぎて置けなければ null', () => {
    expect(thumbFor({ scroll: 0, size: 1000, client: 3 }, 0)).toBeNull()
  })
})

describe('scrollDeltaFor', () => {
  test('つまみを動ける長さぶん動かすと、スクロールの最後まで進む', () => {
    const a = { scroll: 0, size: 1000, client: 500 }
    const t = thumbFor(a, 0)
    if (!t) throw new Error('つまみが無い')
    expect(scrollDeltaFor(a, t, t.travel)).toBeCloseTo(500)
    expect(scrollDeltaFor(a, t, 0)).toBe(0)
  })
})

describe('hasMore / overflows', () => {
  test('先頭は下だけ、途中は両方、末尾は上だけ。はみ出していなければどちらも無い', () => {
    expect(hasMore({ scroll: 0, size: 1000, client: 500 })).toEqual({ before: false, after: true })
    expect(hasMore({ scroll: 200, size: 1000, client: 500 })).toEqual({ before: true, after: true })
    expect(hasMore({ scroll: 500, size: 1000, client: 500 })).toEqual({
      before: true,
      after: false,
    })
    expect(hasMore({ scroll: 0, size: 500, client: 500 })).toEqual({ before: false, after: false })
    expect(overflows({ scroll: 0, size: 500, client: 500 })).toBe(false)
  })
})
