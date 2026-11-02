// 作成エディタ用の軽量リッチテキストのクリーニング。
// 許可する書式: 太字・リンク（http／https／mailto）・箇条書き（順序なし）のみ（F-11）。
const BLOCK_TO_PARAGRAPH = [
  'div',
  'section',
  'article',
  'header',
  'footer',
  'main',
  'nav',
  'aside',
  'address',
  'center',
  'figure',
  'figcaption',
  'pre',
  'blockquote',
  'tr',
  'dt',
  'dd',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
]

export const EDITOR_TAGS = ['p', 'br', 'strong', 'b', 'a', 'ul', 'li']

const ALLOWED_URI = /^(?:https?|mailto):/i

// 中身ごと捨てる要素（テキストとして残ってはいけないもの・名前空間が異なる要素）。
const DROP_WITH_CONTENT = [
  'script',
  'style',
  'head',
  'title',
  'template',
  'noscript',
  'iframe',
  'object',
  'embed',
  'svg',
  'math',
  'textarea',
  'select',
  'button',
  'applet',
  'frame',
  'frameset',
  'audio',
  'video',
  'canvas',
].join(',')

/** 解析のみ行う inert な文書（画像の読み込み等を起こさない）。 */
export function parseBody(html: string): HTMLElement {
  return new DOMParser().parseFromString(html, 'text/html').body
}

function rename(el: Element, tag: string): void {
  const next = el.ownerDocument.createElement(tag)
  while (el.firstChild) next.appendChild(el.firstChild)
  el.replaceWith(next)
}

function rebuild(src: Node, dest: Node, tags: readonly string[]): void {
  const doc = dest.ownerDocument as Document
  for (const child of Array.from(src.childNodes)) {
    if (child.nodeType === 3) {
      dest.appendChild(doc.createTextNode(child.textContent ?? ''))
      continue
    }
    if (child.nodeType !== 1) continue
    const el = child as Element
    if (el.matches(DROP_WITH_CONTENT)) continue
    const tag = el.localName
    const href = el.getAttribute('href')?.trim()
    const safeLink = href !== undefined && ALLOWED_URI.test(href)
    if (!tags.includes(tag) || (tag === 'a' && !safeLink)) {
      rebuild(el, dest, tags)
      continue
    }
    const copy = doc.createElement(tag)
    if (tag === 'a' && href) copy.setAttribute('href', href)
    dest.appendChild(copy)
    rebuild(el, copy, tags)
  }
}

/**
 * HTML を許可タグだけの DOM に作り直して返す（許可リスト方式。属性は a の href（http／https／mailto）のみ）。
 * ブロック要素は段落にして改行を保ち、`ol` は `orderedToBullet` なら箇条書きにする。許可外のタグは外して中身を残す。
 */
export function sanitizeToBody(
  html: string,
  tags: readonly string[],
  orderedToBullet: boolean,
): HTMLElement {
  const src = parseBody(html)
  for (const el of Array.from(src.querySelectorAll(BLOCK_TO_PARAGRAPH.join(',')))) rename(el, 'p')
  if (orderedToBullet) for (const el of Array.from(src.querySelectorAll('ol'))) rename(el, 'ul')
  const out = src.ownerDocument.createElement('body')
  rebuild(src, out, tags)
  return out
}

/**
 * 貼り付け・AI 生成結果などの HTML を、エディタが扱える形（p／br／strong／b／a／ul／li のみ）に変換する。
 * 順序付きリストは箇条書きにする。
 */
export function cleanRichHtml(html: string): string {
  return sanitizeToBody(html, EDITOR_TAGS, true).innerHTML
}

/** リンク URL を http／https／mailto に限定する。スキームなしは https:// を補う。許可外は undefined。 */
export function normalizeLinkUrl(raw: string): string | undefined {
  const url = raw.trim()
  if (url === '' || /\s/.test(url)) return undefined
  const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(url)
  if (scheme) return ALLOWED_URI.test(url) ? url : undefined
  if (/^[^/@\s]+@[^/@\s]+$/.test(url)) return `mailto:${url}`
  if (url.startsWith('//')) return undefined
  return `https://${url}`
}

/** エディタのリンクで許可する URL か。 */
export function isAllowedLink(url: string): boolean {
  return ALLOWED_URI.test(url.trim())
}
