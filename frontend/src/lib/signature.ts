// 署名ブロックの検出・挿入・差し替え（F-07、components/overview.md §3.7）。純粋な文字列⇄DOM 変換のみ。
// Go の signature.Block と同じ形式 `<div data-hitomado-signature data-account-id="…">-- <br>署名</div>` を扱う。
import { parseBody, sanitizeToBody } from './richtext'

export const SIGNATURE_ATTR = 'data-hitomado-signature'
export const QUOTE_ATTR = 'data-hitomado-quote'
export const ACCOUNT_ATTR = 'data-account-id'

const SIGNATURE_SELECTOR = `[${SIGNATURE_ATTR}]`
const QUOTE_SELECTOR = `blockquote[${QUOTE_ATTR}]`

// Go の signature.Sanitize と同じ許可タグ（署名は設定画面のエディタ由来だが、防御的に再度絞る）。
const SIGNATURE_TAGS = ['p', 'br', 'strong', 'b', 'em', 'i', 'u', 'a', 'ul', 'ol', 'li']

export type ReplaceStatus = 'replaced' | 'edited' | 'absent'

export interface SignatureBlockInfo {
  accountId: string
  /** ブロックの内側 HTML（`-- <br>` を含む） */
  content: string
}

export interface ReplaceOptions {
  /** 直前に挿入したブロックの内側 HTML。ブロックの現在内容がこれと（正規化して）一致すれば未編集とみなす。 */
  fromSig: string | undefined
  /** 切替先アカウントの署名 HTML（`-- ` 区切りなし。空なら署名なし）。 */
  toSig: string
  toAccountId: string
}

/** 署名 HTML を許可タグだけにする。空白のみなら ""。 */
export function sanitizeSignature(sigHtml: string): string {
  const body = sanitizeToBody(sigHtml, SIGNATURE_TAGS, false)
  return body.textContent?.trim() === '' ? '' : body.innerHTML.trim()
}

/** ブロック内側の HTML（`-- <br>` ＋署名）。署名が空なら ""。 */
export function signatureContent(sigHtml: string): string {
  const sig = sanitizeSignature(sigHtml)
  return sig === '' ? '' : `-- <br>${sig}`
}

function escapeAttr(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
}

/** 署名ブロック全体の HTML。署名が空なら ""。 */
export function buildSignatureBlock(accountId: string, sigHtml: string): string {
  const content = signatureContent(sigHtml)
  if (content === '') return ''
  return `<div ${SIGNATURE_ATTR} ${ACCOUNT_ATTR}="${escapeAttr(accountId)}">${content}</div>`
}

/** 本文中の最初の署名ブロックを返す。無ければ undefined。 */
export function findSignature(html: string): SignatureBlockInfo | undefined {
  const el = parseBody(html).querySelector(SIGNATURE_SELECTOR)
  if (!el) return undefined
  return { accountId: el.getAttribute(ACCOUNT_ATTR) ?? '', content: el.innerHTML }
}

const BLOCK_TAGS: Record<string, true> = { P: true, UL: true, OL: true, LI: true, DIV: true }

function canonicalize(node: Node, out: string[]): void {
  if (node.nodeType === 3) {
    out.push((node.textContent ?? '').replace(/\s+/g, ' '))
    return
  }
  if (node.nodeType !== 1) return
  const el = node as Element
  const tag = el.tagName
  if (tag === 'BR') {
    out.push('\n')
    return
  }
  const block = BLOCK_TAGS[tag] === true
  if (block) out.push('\n')
  if (tag === 'LI') out.push('• ')
  if (tag === 'STRONG' || tag === 'B') out.push('**')
  if (tag === 'A') out.push('[')
  for (const child of Array.from(el.childNodes)) canonicalize(child, out)
  if (tag === 'A') out.push(`](${el.getAttribute('href') ?? ''})`)
  if (tag === 'STRONG' || tag === 'B') out.push('**')
  if (block) out.push('\n')
}

/**
 * 署名の比較用に正規化する（許可タグだけに作り直した HTML を、エディタの再シリアライズで変わる差を吸収した文字列にする）。
 * 段落と `<br>` の違い・行末の空白・斜体／下線・順序付きリストの違いは区別しない。
 */
export function normalizeSignature(html: string): string {
  const out: string[] = []
  for (const child of Array.from(sanitizeToBody(html, SIGNATURE_TAGS, false).childNodes)) {
    canonicalize(child, out)
  }
  return out
    .join('')
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')
    .join('\n')
}

function topLevelAncestor(body: HTMLElement, el: Element): Node {
  let node: Node = el
  while (node.parentNode && node.parentNode !== body) node = node.parentNode
  return node
}

function toNode(doc: Document, html: string): Node {
  const tpl = parseBody(html)
  return doc.importNode(tpl.firstChild as Node, true)
}

/** 署名ブロックを（あれば）差し替え、無ければ引用の直前（引用が無ければ末尾）に挿入する。署名が空ならブロックを取り除く。 */
export function applySignature(html: string, accountId: string, sigHtml: string): string {
  const body = parseBody(html)
  const doc = body.ownerDocument
  const existing = body.querySelector(SIGNATURE_SELECTOR)
  const block = buildSignatureBlock(accountId, sigHtml)
  if (block === '') {
    existing?.remove()
    return body.innerHTML
  }
  const node = toNode(doc, block)
  if (existing) {
    existing.replaceWith(node)
    return body.innerHTML
  }
  const quote = body.querySelector(QUOTE_SELECTOR)
  if (quote) body.insertBefore(node, topLevelAncestor(body, quote))
  else body.appendChild(node)
  return body.innerHTML
}

/**
 * 送信元切替時の署名差し替え。確認は呼び出し側が行う:
 * - `replaced`: ブロックが未編集（または空）だったので差し替え（署名なしへの切替はブロック削除）済みの html を返す。
 * - `edited`: 利用者が編集済み。html は変更しない。確認が取れたら {@link applySignature} を呼ぶ。
 * - `absent`: ブロックが無い。html は変更しない。
 */
export function replaceSignature(
  html: string,
  opts: ReplaceOptions,
): { html: string; status: ReplaceStatus } {
  const current = findSignature(html)
  if (!current) return { html, status: 'absent' }
  const currentNorm = normalizeSignature(current.content)
  const untouched =
    currentNorm === '' ||
    (opts.fromSig !== undefined && currentNorm === normalizeSignature(opts.fromSig))
  if (!untouched) return { html, status: 'edited' }
  return { html: applySignature(html, opts.toAccountId, opts.toSig), status: 'replaced' }
}

function isMarked(node: Node): boolean {
  if (node.nodeType !== 1) return false
  const el = node as Element
  return el.matches(SIGNATURE_SELECTOR) || el.matches(QUOTE_SELECTOR)
}

function splitAtTail(html: string): { body: HTMLElement; head: Node[]; tail: Node[] } {
  const body = parseBody(html)
  const nodes = Array.from(body.childNodes)
  const at = nodes.findIndex(isMarked)
  return at < 0
    ? { body, head: nodes, tail: [] }
    : { body, head: nodes.slice(0, at), tail: nodes.slice(at) }
}

function serialize(nodes: Node[]): string {
  const div = nodes.length > 0 ? (nodes[0].ownerDocument as Document).createElement('div') : null
  if (!div) return ''
  for (const n of nodes) div.appendChild(n.cloneNode(true))
  return div.innerHTML
}

/** 署名・引用より前（利用者が書く部分）に文字があるか。 */
export function hasUserBody(html: string): boolean {
  return splitAtTail(html).head.some((n) => (n.textContent ?? '').trim() !== '')
}

/** 署名・引用を保ったまま、それより前の本文を `bodyHtml` に置き換える。bodyHtml は貼り付けと同じ規則で整形済みであること。 */
export function replaceUserBody(html: string, bodyHtml: string): string {
  const { tail } = splitAtTail(html)
  return bodyHtml + serialize(tail)
}

/** 署名または引用が先頭にあるとき、入力位置のために空の段落を先頭に足す。 */
export function ensureLeadingParagraph(html: string): string {
  const first = Array.from(parseBody(html).childNodes).find(
    (n) => n.nodeType !== 3 || (n.textContent ?? '').trim() !== '',
  )
  return first && isMarked(first) ? `<p></p>${html}` : html
}
