// 宛先入力の補助（貼り付けの複数アドレス分割・チップの形式表示）。
// 検証の正は Go（PrepareSend の INVALID_ADDRESS）。ここは入力の分割と、明らかな誤りの強調だけを担う。
import type { Address } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'

const LIKELY_EMAIL = /^[^\s@<>,;"]+@[^\s@<>,;"]+$/

export function isLikelyEmail(email: string): boolean {
  return LIKELY_EMAIL.test(email)
}

/** 引用符・山括弧の外にある `,` `;` 改行で分割する。 */
function splitTopLevel(text: string): string[] {
  const parts: string[] = []
  let cur = ''
  let quoted = false
  let angle = false
  for (const ch of text) {
    if (ch === '"') quoted = !quoted
    else if (!quoted && ch === '<') angle = true
    else if (!quoted && ch === '>') angle = false
    if (!quoted && !angle && (ch === ',' || ch === ';' || ch === '\n' || ch === '\r')) {
      parts.push(cur)
      cur = ''
    } else {
      cur += ch
    }
  }
  parts.push(cur)
  return parts
}

function unquote(name: string): string {
  const n = name.trim()
  return n.length >= 2 && n.startsWith('"') && n.endsWith('"') ? n.slice(1, -1).trim() : n
}

function toAddress(email: string, name?: string): Address {
  const addr: Address = { email: email.trim().toLowerCase() }
  if (name) addr.name = name
  return addr
}

/**
 * 貼り付け・入力された文字列をアドレスに分割する。`Name <a@x>`、カンマ／セミコロン／改行／空白区切りに対応。
 * 空要素は捨てる。形式の判定はしない（形式不正は {@link isLikelyEmail} で表示し、最終検証は Go が行う）。
 */
export function parseAddresses(text: string): Address[] {
  const out: Address[] = []
  for (const raw of splitTopLevel(text)) {
    const part = raw.trim()
    if (part === '') continue
    const m = /^(.*)<([^<>]*)>\s*$/.exec(part)
    if (m) {
      const email = m[2].trim()
      if (email !== '') out.push(toAddress(email, unquote(m[1])))
      continue
    }
    for (const token of part.split(/\s+/)) if (token !== '') out.push(toAddress(token))
  }
  return out
}

/** 既存の宛先に重複（大文字小文字を区別しない）なく追加する。 */
export function mergeAddresses(current: readonly Address[], added: readonly Address[]): Address[] {
  const seen = new Set(current.map((a) => a.email.toLowerCase()))
  const out = [...current]
  for (const a of added) {
    const key = a.email.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(a)
  }
  return out
}

/** チップの表示名。表示名があれば `名前 <アドレス>`、無ければアドレス。 */
export function addressLabel(a: Address): string {
  return a.name ? `${a.name} <${a.email}>` : a.email
}
