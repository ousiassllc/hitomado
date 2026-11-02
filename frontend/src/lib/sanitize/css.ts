// メール本文の CSS サニタイズ（lib/sanitize から使う純 TS。DOM に依存しない）。
// 方針（components/overview.md §3.8）: インラインスタイルは許可するが、`url()` を含む宣言は除去する。
// リモート画像の表示を許可したときのみ `https:` の url() を残す。data: 画像は常に残す。

export interface CssPolicy {
  /** true のとき https: の url() を残す（メッセージ単位の「画像を表示」） */
  allowRemoteImages: boolean
}

const DATA_IMAGE = /^data:image\/(?:png|jpe?g|gif|webp|bmp|avif|x-icon);/i
const HTTPS_URL = /^https:\/\//i
// 実行・外部読み込み・ローダー系。見つかった宣言は丸ごと除去する。
const BLOCKED =
  /expression\s*\(|behaviou?r\s*:|-moz-binding|@import|javascript:|vbscript:|image-set\s*\(|\bsrc\s*\(|\bimage\s*\(|\belement\s*\(|\bpaint\s*\(/i
// 入れ子のルールを持つ at-rule（本体をルールとして再帰処理する）
const NESTED_AT = /^@(?:-\w+-)?(?:media|supports|layer|container|document|keyframes)\b/i
// 本体を宣言として扱う at-rule
const DECL_AT = /^@page\b/i
const MAX_DEPTH = 8

function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?(?:\*\/|$)/g, '')
}

/** CSS のエスケープ（\75 や \u）を解いた文字列。検査専用で、出力には使わない。 */
function decodeEscapes(css: string): string {
  return css
    .replace(/\\([0-9a-fA-F]{1,6})[ \t\r\n\f]?/g, (_, hex: string) => {
      const cp = Number.parseInt(hex, 16)
      return cp > 0 && cp <= 0x10ffff ? String.fromCodePoint(cp) : ''
    })
    .replace(/\\(\r\n|[\s\S])/g, (_, c: string) => (c === '\n' || c === '\r\n' ? '' : c))
}

function urlCount(s: string): number {
  return s.match(/url\(/gi)?.length ?? 0
}

function isAllowedUrl(raw: string, policy: CssPolicy): boolean {
  // biome-ignore lint/suspicious/noControlCharactersInRegex: 制御文字・空白を除いて検査する
  const v = raw.replace(/[\u0000-\u0020\u007f]/g, '')
  if (DATA_IMAGE.test(v)) return true
  return policy.allowRemoteImages && HTTPS_URL.test(v)
}

const WS = /[\s]/

function skipWs(s: string, from: number): number {
  let i = from
  while (i < s.length && WS.test(s[i] as string)) i++
  return i
}

/**
 * 宣言中の `url(` をすべて手書きでスキャンして検査する（線形時間。正規表現の入れ子量指定子は使わない）。
 * `)` で閉じない `url(`・引用符が閉じない／閉じ括弧の前に余計な文字がある `url(` が 1 つでもあれば false。
 * 失敗したら即座に打ち切るので、不正な入力でも全体で O(n)。
 */
function urlsAllowed(decoded: string, policy: CssPolicy): boolean {
  const re = /url\(/gi
  let from = 0
  for (;;) {
    re.lastIndex = from
    const m = re.exec(decoded)
    if (!m) return true
    let i = skipWs(decoded, m.index + 4)
    let value: string
    const q = decoded[i]
    if (q === '"' || q === "'") {
      const end = decoded.indexOf(q, i + 1)
      if (end < 0) return false
      value = decoded.slice(i + 1, end)
      i = skipWs(decoded, end + 1)
      if (decoded[i] !== ')') return false
    } else {
      const end = decoded.indexOf(')', i)
      if (end < 0) return false
      value = decoded.slice(i, end)
      i = end
    }
    if (!isAllowedUrl(value, policy)) return false
    from = i + 1
  }
}

function isSafeDeclaration(decl: string, policy: CssPolicy): boolean {
  const decoded = decodeEscapes(decl)
  if (BLOCKED.test(decoded)) return false
  const n = urlCount(decoded)
  if (n === 0) return true
  // エスケープで url( を隠した宣言は除去する
  if (urlCount(decl) !== n) return false
  return urlsAllowed(decoded, policy)
}

/** 引用符・括弧の外にある sep で分割する。 */
function splitTopLevel(text: string, sep: string): string[] {
  const out: string[] = []
  let depth = 0
  let quote = ''
  let start = 0
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (c === '\\') {
      i++
    } else if (quote) {
      if (c === quote) quote = ''
    } else if (c === '"' || c === "'") {
      quote = c
    } else if (c === '(') {
      depth++
    } else if (c === ')') {
      depth = Math.max(0, depth - 1)
    } else if (c === sep && depth === 0) {
      out.push(text.slice(start, i))
      start = i + 1
    }
  }
  out.push(text.slice(start))
  return out
}

/** インライン style 属性・ルール本体の宣言リストをサニタイズする。 */
export function sanitizeDeclarations(css: string, policy: CssPolicy): string {
  const kept = splitTopLevel(stripComments(css), ';')
    .map((d) => d.trim())
    .filter((d) => d !== '' && !d.includes('<') && isSafeDeclaration(d, policy))
  return kept.join('; ')
}

/** プレリュード（`{` または `;` まで）の終端を返す。term は終端文字（無ければ ''）。 */
function scanPrelude(src: string, from: number): { end: number; term: string } {
  let depth = 0
  let quote = ''
  for (let i = from; i < src.length; i++) {
    const c = src[i]
    if (c === '\\') {
      i++
    } else if (quote) {
      if (c === quote) quote = ''
    } else if (c === '"' || c === "'") {
      quote = c
    } else if (c === '(' || c === '[') {
      depth++
    } else if (c === ')' || c === ']') {
      depth = Math.max(0, depth - 1)
    } else if (depth === 0 && (c === '{' || c === ';' || c === '}')) {
      return { end: i, term: c }
    }
  }
  return { end: src.length, term: '' }
}

/** open の `{` に対応する `}` の位置（無ければ末尾）。 */
function findBlockEnd(src: string, open: number): number {
  let depth = 0
  let quote = ''
  for (let i = open; i < src.length; i++) {
    const c = src[i]
    if (c === '\\') {
      i++
    } else if (quote) {
      if (c === quote) quote = ''
    } else if (c === '"' || c === "'") {
      quote = c
    } else if (c === '{') {
      depth++
    } else if (c === '}' && --depth === 0) {
      return i
    }
  }
  return src.length
}

function processRules(src: string, policy: CssPolicy, depth: number): string {
  if (depth > MAX_DEPTH) return ''
  let out = ''
  let i = 0
  while (i < src.length) {
    const { end, term } = scanPrelude(src, i)
    if (term !== '{') {
      // `@import ...;`・`@charset ...;`・孤立した `}` は捨てる
      i = end + 1
      continue
    }
    const prelude = src.slice(i, end).trim()
    const close = findBlockEnd(src, end)
    const body = src.slice(end + 1, close)
    i = close + 1
    if (prelude.startsWith('@')) {
      if (NESTED_AT.test(prelude)) out += `${prelude}{${processRules(body, policy, depth + 1)}}\n`
      else if (DECL_AT.test(prelude)) out += `${prelude}{${sanitizeDeclarations(body, policy)}}\n`
      // @font-face など上記以外の at-rule は捨てる
      continue
    }
    out += `${prelude}{${sanitizeDeclarations(body, policy)}}\n`
  }
  return out
}

/** `<style>` 要素の中身（スタイルシート全体）をサニタイズする。 */
export function sanitizeStyleSheet(css: string, policy: CssPolicy): string {
  return processRules(stripComments(css).replaceAll('<', ''), policy, 0)
}
