// UI から青をなくす回帰テスト（#389）。--focus トークンの廃止、フォーカスリング（:focus-visible だけ・--accent）、::selection（色味なし）を確かめる。
// 例外: アカウント識別色（--account-<key>。ユーザーが選ぶデータの色で、palette のキーと対応する。docs/components/overview.md §5.3）。
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { parse } from '@vue/compiler-sfc'

const srcDir = new URL('../../src/', import.meta.url).pathname

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = join(dir, e.name)
    return e.isDirectory() ? walk(p) : [p]
  })
}

const files = walk(srcDir).filter((f) => !/\.test\.ts$/.test(f))

/** <style> の中身（.vue）と .css の全文。コメントは除く。 */
function styles(): { file: string; css: string }[] {
  return files.flatMap((file) => {
    if (file.endsWith('.css')) return [{ file, css: readFileSync(file, 'utf8') }]
    if (!file.endsWith('.vue')) return []
    return parse(readFileSync(file, 'utf8')).descriptor.styles.map((s) => ({
      file,
      css: s.content,
    }))
  })
}

const stripComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, '')

function hueOf(hex: string): { h: number; s: number } {
  const v = hex.length === 3 ? [...hex].map((c) => c + c).join('') : hex
  const [r, g, b] = [0, 2, 4].map((i) => Number.parseInt(v.slice(i, i + 2), 16) / 255)
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const d = max - min
  if (d === 0) return { h: 0, s: 0 }
  const l = (max + min) / 2
  const s = d / (1 - Math.abs(2 * l - 1))
  const h = max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return { h: (h * 60 + 360) % 360, s }
}

describe('--focus トークンの廃止（#389）', () => {
  test('コードにもスタイルにも --focus が残っていない', () => {
    const offenders = files
      .filter((f) => /\.(vue|css|ts|html)$/.test(f))
      .filter((f) => /--focus\b/.test(readFileSync(f, 'utf8')))
    expect(offenders).toEqual([])
  })

  test('index.html にも残っていない', () => {
    const html = readFileSync(new URL('../../index.html', import.meta.url), 'utf8')
    expect(html).not.toMatch(/--focus\b/)
  })
})

describe('青系の色がない（#389。アカウント識別色 --account-* は除く）', () => {
  test('スタイルの 16 進・rgb()・hsl()・色名に、青系（色相 190〜270°・彩度 25% 以上）の値がない', () => {
    const offenders: string[] = []
    for (const { file, css } of styles()) {
      for (const line of stripComments(css).split('\n')) {
        if (/--account-/.test(line)) continue
        for (const m of line.matchAll(/#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b/g)) {
          const { h, s } = hueOf(m[1])
          if (h >= 190 && h <= 270 && s >= 0.25) offenders.push(`${file}: ${m[0]}`)
        }
        for (const m of line.matchAll(/rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)/g)) {
          const hex = [m[1], m[2], m[3]]
            .map((n) => Number(n).toString(16).padStart(2, '0'))
            .join('')
          const { h, s } = hueOf(hex)
          if (h >= 190 && h <= 270 && s >= 0.25) offenders.push(`${file}: ${m[0]}`)
        }
        for (const m of line.matchAll(/hsla?\(\s*(\d+)(?:deg)?[\s,]+(\d+)%/g)) {
          if (Number(m[1]) >= 190 && Number(m[1]) <= 270 && Number(m[2]) >= 25)
            offenders.push(`${file}: ${m[0]}`)
        }
        if (
          /:\s*[^;]*\b(?:blue|navy|royalblue|dodgerblue|cornflowerblue|steelblue|skyblue|indigo)\b/i.test(
            line,
          )
        )
          offenders.push(`${file}: ${line.trim()}`)
      }
    }
    expect(offenders).toEqual([])
  })

  test('トークンの状態色（--info など）も青系でない', () => {
    const tokens = readFileSync(join(srcDir, 'styles/tokens.css'), 'utf8')
    const colors = [...tokens.matchAll(/^\s*--(?!account-)[\w-]+:\s*#([0-9a-fA-F]{6})\b/gm)]
    expect(colors.length).toBeGreaterThan(0)
    for (const m of colors) {
      const { h, s } = hueOf(m[1])
      expect(h >= 190 && h <= 270 && s >= 0.25).toBe(false)
    }
  })
})

describe('フォーカスリングは :focus-visible のときだけ、--accent で出る（#389）', () => {
  const rules = styles().flatMap(({ file, css }) =>
    [...stripComments(css).matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => ({
      file,
      selector: m[1].trim(),
      body: m[2],
    })),
  )

  test('outline を描くルール（none を除く）は、セレクタが :focus-visible を含む', () => {
    const offenders = rules
      .filter((r) => /outline(?!-offset):\s*(?!none)\S/.test(r.body))
      .filter((r) => !/:focus-visible|file-drop-target-active/.test(r.selector)) // ファイルのドロップ先の表示（フォーカスではない）
      .map((r) => `${r.file}: ${r.selector}`)
    expect(offenders).toEqual([])
  })

  test('フォーカスの枠色・リングを、:focus・:focus-within だけで出していない', () => {
    const offenders = rules
      .filter((r) => /:focus(?!-visible)/.test(r.selector))
      .filter((r) => /outline(?!-offset):\s*(?!none)\S|border-color|box-shadow/.test(r.body))
      .map((r) => `${r.file}: ${r.selector}`)
    expect(offenders).toEqual([])
  })

  test('フォーカスのリングと枠色は --accent（outline: <n>px solid var(--accent)）', () => {
    const rings = rules.filter(
      (r) => /:focus-visible/.test(r.selector) && /outline(?!-offset):\s*(?!none)\S/.test(r.body),
    )
    expect(rings.length).toBeGreaterThan(10)
    for (const r of rings) {
      expect(r.body).toMatch(/outline:\s*\d+px solid var\(--accent\);/)
    }
  })
})

describe('::selection は色味なし（#389）', () => {
  const reset = stripComments(readFileSync(join(srcDir, 'styles/reset.css'), 'utf8'))
  const tokens = readFileSync(join(srcDir, 'styles/tokens.css'), 'utf8')

  test('地は --accent（ライト＝インク #050505・ダーク＝ボーン色 #f5f5f4）の 20〜25%。文字色は指定しない', () => {
    const body = reset.match(/::selection\s*\{([^}]*)\}/)?.[1] ?? ''
    const m = body.match(/background:\s*color-mix\(in srgb, var\(--accent\) (\d+)%, transparent\);/)
    expect(m).not.toBeNull()
    expect(Number(m?.[1])).toBeGreaterThanOrEqual(20)
    expect(Number(m?.[1])).toBeLessThanOrEqual(25)
    expect(body).not.toMatch(/(^|[;\s])color:/)
  })

  test('--accent の値が無彩色（ライト #050505・ダーク #f5f5f4）なので、選択色は両テーマとも色味なし', () => {
    const light = tokens.match(/:root\s*\{[\s\S]*?--accent:\s*(#[0-9a-f]{6});/)?.[1]
    const dark = tokens.match(
      /prefers-color-scheme: dark\)\s*\{[\s\S]*?--accent:\s*(#[0-9a-f]{6});/,
    )?.[1]
    expect(light).toBe('#050505')
    expect(dark).toBe('#f5f5f4')
    for (const hex of [light, dark]) {
      const [r, g, b] = [1, 3, 5].map((i) => Number.parseInt((hex ?? '').slice(i, i + 2), 16))
      expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThanOrEqual(1)
    }
  })

  test('ほかに ::selection を上書きするスタイルがない', () => {
    const others = styles().filter(
      ({ file, css }) => !file.endsWith('styles/reset.css') && /::selection/.test(css),
    )
    expect(others).toEqual([])
  })
})
