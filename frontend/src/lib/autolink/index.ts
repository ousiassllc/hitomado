// 本文の素の URL・メールアドレスを見つけてリンクにする（#312。components/overview.md §3.8 項 9、NF-S07）。
// 純 TS。見つけるのは http／https／www.／メールアドレス（mailto）だけで、javascript: などは候補にしない。
// 文字種は ASCII の URL 文字だけなので、URL の直後に続く日本語・全角の句読点・かっこは巻き込まない。

export interface AutoLink {
  /** 元の文字列での開始位置（含む） */
  start: number
  /** 元の文字列での終了位置（含まない） */
  end: number
  /** リンク先。http／https／mailto のいずれか */
  href: string
}

// URL に使える文字（RFC 3986 の予約文字＋非予約文字＋%）。空白・<>"・非 ASCII を含めない。
const URL_CHARS = "A-Za-z0-9\\-._~:/?#\\[\\]@!$&'()*+,;=%"
const SCHEME_URL = `https?:\\/\\/[${URL_CHARS}]+`
// "www." は直前が英数字・. - / @ のときは別の語の一部なので対象にしない。
const WWW_URL = `(?<![A-Za-z0-9._%+@/-])www\\.[${URL_CHARS}]+`
// ローカル部は直前が同じ文字種のときは途中から始めない。ドメインは 1 つ以上のドット区切りで、末尾（TLD）は英字 2 文字以上。
const EMAIL = '(?<![A-Za-z0-9._%+-])[A-Za-z0-9._%+-]+@(?:[A-Za-z0-9-]+\\.)+[A-Za-z]{2,}'
// 先に当たるものが優先（https://user@host の @ をメールとして拾わないよう、URL を先に置く）。
const CANDIDATE = new RegExp(`${SCHEME_URL}|${WWW_URL}|${EMAIL}`, 'g')

const CLOSERS: Record<string, string> = { ')': '(', ']': '[', '}': '{' }
// 文末の句読点として URL に含めない文字
const TRAILING_PUNCT: Record<string, true> = {
  '.': true,
  ',': true,
  ';': true,
  ':': true,
  '!': true,
  '?': true,
  "'": true,
  '*': true,
}

function count(s: string, ch: string): number {
  let n = 0
  for (const c of s) if (c === ch) n++
  return n
}

/** 末尾の句読点と、対応する開きかっこのない閉じかっこを落とす。 */
function trimTrailing(s: string): string {
  let out = s
  for (;;) {
    const last = out.at(-1)
    if (last === undefined) return out
    const open = CLOSERS[last]
    if (
      TRAILING_PUNCT[last] === true ||
      (open !== undefined && count(out, last) > count(out, open))
    ) {
      out = out.slice(0, -1)
      continue
    }
    return out
  }
}

/** ホストが空でなく、スキームが http／https のときだけ true。"https://" や "https://." のような殻を弾く。 */
function hasHost(href: string): boolean {
  try {
    const u = new URL(href)
    return (u.protocol === 'http:' || u.protocol === 'https:') && /[A-Za-z0-9]/.test(u.hostname)
  } catch {
    return false
  }
}

function toLink(start: number, raw: string): AutoLink | undefined {
  // EMAIL は末尾が英字 2 文字以上のドメインで終わるので、トリムは要らない。
  if (!/^(?:https?:\/\/|www\.)/i.test(raw))
    return { start, end: start + raw.length, href: `mailto:${raw}` }
  const text = trimTrailing(raw)
  const href = /^www\./i.test(text) ? `https://${text}` : text
  return hasHost(href) ? { start, end: start + text.length, href } : undefined
}

/** テキストの中の URL・メールアドレスを、出現順・重なりなしで返す。 */
export function findLinks(text: string): AutoLink[] {
  const out: AutoLink[] = []
  for (const m of text.matchAll(CANDIDATE)) {
    const link = toLink(m.index, m[0])
    if (link) out.push(link)
  }
  return out
}

export type TextSegment = { text: string; href?: string }

/** テキストをリンクの部分とそれ以外に分ける（連結すると元の文字列に戻る）。テキストメールの表示用。 */
export function splitLinks(text: string): TextSegment[] {
  const out: TextSegment[] = []
  let pos = 0
  for (const l of findLinks(text)) {
    if (l.start > pos) out.push({ text: text.slice(pos, l.start) })
    out.push({ text: text.slice(l.start, l.end), href: l.href })
    pos = l.end
  }
  if (pos < text.length) out.push({ text: text.slice(pos) })
  return out
}

/** リンクの中・リンクにしてはいけない要素の中のテキストは触らない。 */
const SKIP_TAGS: Record<string, true> = {
  a: true,
  area: true,
  script: true,
  style: true,
  textarea: true,
  button: true,
  title: true,
}

function inSkipped(node: Node, root: Node): boolean {
  for (let el = node.parentElement; el && el !== root; el = el.parentElement) {
    if (SKIP_TAGS[el.localName] === true) return true
  }
  return false
}

/**
 * DOM の中のテキストノードに含まれる素の URL・メールアドレスを `<a href rel>` にする（破壊的）。
 * すでに `<a>` の中にあるテキスト（href を除去されたものを含む）は変えない。生成する a は `href`（http／https／
 * mailto）と `rel="noopener noreferrer"` だけで、`target` は付けない（クリックは MessageBody が捕捉して開く）。
 */
export function autolinkDom(root: Element): void {
  const doc = root.ownerDocument
  // 走査中に木を変えないよう、先にテキストノードを集める。
  const texts: Text[] = []
  const walker = doc.createTreeWalker(root, 4 /* NodeFilter.SHOW_TEXT */)
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!inSkipped(n, root)) texts.push(n as Text)
  }
  for (const node of texts) {
    const segments = splitLinks(node.data)
    if (!segments.some((s) => s.href)) continue
    const frag = doc.createDocumentFragment()
    for (const s of segments) {
      if (!s.href) {
        frag.append(doc.createTextNode(s.text))
        continue
      }
      const a = doc.createElement('a')
      a.setAttribute('href', s.href)
      a.setAttribute('rel', 'noopener noreferrer')
      a.textContent = s.text
      frag.append(a)
    }
    node.replaceWith(frag)
  }
}
