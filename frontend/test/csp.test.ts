// NF-S06: WebView の CSP（frontend/index.html の meta）が外部オリジンの読み込みを許さないことを固定する。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
const meta = html.match(/<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]*)"/)

// ディレクティブ名 → ソースの並び
function parseCsp(content: string): Map<string, string[]> {
  const directives = new Map<string, string[]>()
  for (const part of content.split(';')) {
    const [name, ...sources] = part.trim().split(/\s+/)
    if (name) directives.set(name, sources)
  }
  return directives
}

const csp = parseCsp(meta?.[1] ?? '')

describe('index.html の CSP（NF-S06）', () => {
  test('CSP の meta が index.html にある', () => {
    expect(meta).not.toBeNull()
    expect(csp.size).toBeGreaterThan(0)
  })

  test("script-src は 'self' のみ（unsafe-inline・unsafe-eval・https:・ワイルドカードを許さない）", () => {
    expect(csp.get('script-src')).toEqual(["'self'"])
  })

  test("connect-src は 'self' のみ（外部オリジンへの fetch・WebSocket を許さない。外部通信は Go 経由）", () => {
    expect(csp.get('connect-src')).toEqual(["'self'"])
  })

  test("object-src・base-uri・form-action は 'none'", () => {
    for (const name of ['object-src', 'base-uri', 'form-action']) {
      expect(csp.get(name)).toEqual(["'none'"])
    }
  })

  // img-src の https: は、利用者が「画像を表示」を押したときに、メール本文の iframe（srcdoc）がリモート画像を取得するために必要。
  // 親の CSP が srcdoc の iframe にも掛かるので、親の img-src で許す必要がある。
  test('https: を許すのは img-src だけ。他のディレクティブにワイルドカード・外部オリジンを入れない', () => {
    expect(csp.get('img-src')).toContain('https:')
    for (const [name, sources] of csp) {
      if (name === 'img-src') continue
      for (const source of sources) {
        expect(`${name} ${source}`).not.toMatch(/https?:|\*|unsafe-eval/)
      }
    }
  })

  test("default-src は 'self' のみ", () => {
    expect(csp.get('default-src')).toEqual(["'self'"])
  })
})
