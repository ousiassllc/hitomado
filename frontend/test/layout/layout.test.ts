import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
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

  test('ブレイクポイントは 1 か所の定数（ガイドの --bp-sm・--bp-lg）。3 ペインはデスクトップの最小幅（960px）以上', () => {
    expect(BREAKPOINT_WIDE).toBe(1024)
    expect(BREAKPOINT_MEDIUM).toBe(640)
    expect(BREAKPOINT_WIDE).toBeGreaterThanOrEqual(MIN_WINDOW_WIDTH)
    expect(BREAKPOINT_MEDIUM).toBeLessThan(BREAKPOINT_WIDE)
    expect(WIDE_QUERY).toBe(`(min-width: ${BREAKPOINT_WIDE}px)`)
    expect(MEDIUM_QUERY).toBe(`(min-width: ${BREAKPOINT_MEDIUM}px)`)
  })
})

// CSS の @media には --bp-* を書けない（カスタムプロパティは条件に使えない）ので幅を直接書く。値がずれないよう、
// コンポーネントの @media の幅が layout.ts の定数と同じ値だけであることを確かめる（docs §5.3 #284）。
describe('CSS の @media の幅は layout.ts の定数と同じ', () => {
  test('コンポーネントの @media (min/max-width) は 640・1024 のどちらか', () => {
    const allowed = new Set([BREAKPOINT_MEDIUM, BREAKPOINT_WIDE])
    const root = new URL('../../src/', import.meta.url)
    const files = readdirSync(fileURLToPath(root), { recursive: true })
      .map(String)
      .filter((f) => /\.(vue|css)$/.test(f))
    const widths = new Set<number>()
    for (const f of files) {
      const text = readFileSync(new URL(f, root), 'utf8')
      for (const m of text.matchAll(/@media\s*\(\s*(?:min|max)-width:\s*(\d+)px\s*\)/g))
        widths.add(Number(m[1]))
    }
    expect(widths.size).toBeGreaterThan(0)
    for (const w of widths) expect(allowed.has(w)).toBe(true)
  })
})
