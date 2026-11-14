// メール本文のサニタイズと iframe 表示用の srcdoc 生成（components/overview.md §3.8、NF-S04/S05/S07）。
// 純 TS。DOM は DOMPurify が使う window（ブラウザ／テストでは happy-dom）に依存する。
import createPurify, { type Config, type DOMPurify } from 'dompurify'
import { autolinkDom } from '../autolink'
import { frameRootAttr, frameScrollbarCss } from '../overlayScroll/frame'
import { type CssPolicy, sanitizeDeclarations, sanitizeStyleSheet } from './css'

export interface SanitizeOptions {
  /**
   * true のとき https: のリモート画像（img[src]・background 属性・CSS url()）を残す。
   * メッセージ単位の「画像を表示」で、元の HTML を再サニタイズして復元する（既定 false）。
   */
  allowRemoteImages?: boolean
  /**
   * true のとき、本文の素の URL・メールアドレスを `<a>` にする（#312。既存の `<a>` の中は変えない）。
   * サニタイズ後の DOM に対して行うので、生成するリンクは http／https／mailto だけ（既定 false）。
   */
  autolink?: boolean
}

const DATA_IMAGE = /^data:image\/(?:png|jpe?g|gif|webp|bmp|avif|x-icon);/i
const HTTPS_URL = /^https:\/\//i
const OPENABLE_PROTOCOLS: Record<string, true> = { 'http:': true, 'https:': true, 'mailto:': true }

const CONFIG: Config = {
  USE_PROFILES: { html: true }, // svg・MathML は使わない
  FORCE_BODY: true, // 先頭の <style> を head に追い出されないようにする
  FORBID_TAGS: [
    'script',
    'iframe',
    'frame',
    'frameset',
    'object',
    'embed',
    'applet',
    'form',
    'input',
    'button',
    'textarea',
    'select',
    'option',
    'optgroup',
    'fieldset',
    'link',
    'base',
    'meta',
    'video',
    'audio',
    'source',
    'track',
    'picture',
    'canvas',
    'dialog',
    'template',
    'map',
    'area',
  ],
  FORBID_ATTR: ['srcset', 'poster', 'action', 'formaction', 'ping', 'target', 'usemap'],
  // リンク（a[href]・area[href]）の制限は ALLOWED_URI_REGEXP ではなくフックで行う
  // （ALLOWED_URI_REGEXP は width・align など URI 以外の属性値にも適用され、全部消えてしまう）。
  ALLOW_DATA_ATTR: false,
  ALLOW_ARIA_ATTR: false,
}

let purify: DOMPurify | undefined
// フックは 1 回の sanitize 呼び出しの間だけ有効なポリシーを参照する（同期処理）。
let policy: CssPolicy = { allowRemoteImages: false }

function isAllowedImageUrl(raw: string): boolean {
  const v = stripUrl(raw)
  return DATA_IMAGE.test(v) || (policy.allowRemoteImages && HTTPS_URL.test(v))
}

// 制御文字・空白を除いて検査する（"jav\tascript:" 等の回避を許さない）
function stripUrl(raw: string): string {
  // biome-ignore lint/suspicious/noControlCharactersInRegex: 制御文字・空白を除いて検査する
  return raw.replace(/[\u0000-\u0020\u007f]/g, '')
}

const OPENABLE_HREF = /^(?:https?|mailto):/i

function cleanElement(el: Element): void {
  const style = el.getAttribute('style')
  if (style !== null) {
    const clean = sanitizeDeclarations(style, policy)
    if (clean === '') el.removeAttribute('style')
    else el.setAttribute('style', clean)
  }
  for (const name of ['src', 'background']) {
    const v = el.getAttribute(name)
    if (v !== null && !isAllowedImageUrl(v)) el.removeAttribute(name)
  }
  // リンクは http／https／mailto のみ（相対・フラグメント・tel・javascript・file などは href ごと除去）
  if ((el.localName === 'a' || el.localName === 'area') && el.hasAttribute('href')) {
    if (OPENABLE_HREF.test(stripUrl(el.getAttribute('href') ?? '')))
      el.setAttribute('rel', 'noopener noreferrer')
    else el.removeAttribute('href')
  }
}

function instance(): DOMPurify {
  if (purify) return purify
  const p = createPurify(window)
  p.addHook('uponSanitizeElement', (node, data) => {
    if (data.tagName === 'style')
      node.textContent = sanitizeStyleSheet(node.textContent ?? '', policy)
  })
  p.addHook('afterSanitizeAttributes', (node) => {
    if (node.nodeType === 1) cleanElement(node as Element)
  })
  purify = p
  return p
}

/**
 * メール HTML をサニタイズして表示用の HTML 断片を返す。
 * 除去: script・form 系・iframe・object・embed・link・base・meta（refresh）・on* 属性・srcset・
 * url() を含む CSS 宣言（許可時は https: を除く）。サニタイズ不能な環境では空文字を返す（fail closed）。
 */
export function sanitizeEmailHtml(html: string, opts: SanitizeOptions = {}): string {
  const p = instance()
  if (!p.isSupported) return ''
  policy = { allowRemoteImages: opts.allowRemoteImages === true }
  try {
    if (opts.autolink !== true) return String(p.sanitize(html, CONFIG))
    const body = p.sanitize(html, { ...CONFIG, RETURN_DOM: true }) as HTMLElement
    autolinkDom(body)
    return body.innerHTML
  } finally {
    policy = { allowRemoteImages: false }
  }
}

/** iframe 内の Content-Security-Policy。画像表示を許可したときのみ img-src に https: を足す。 */
export function buildCsp(allowImages: boolean): string {
  const img = allowImages ? 'data: https:' : 'data:'
  return `default-src 'none'; img-src ${img}; style-src 'unsafe-inline'`
}

// iframe 内の基本スタイル。色はシステム色（Canvas/CanvasText）で、メール本文が上書きできる。
const BASE_STYLE =
  ':root{color-scheme:light}html{height:auto}body{margin:0;padding:0;background:Canvas;color:CanvasText;' +
  'font:14px/1.5 sans-serif;overflow-wrap:anywhere}img{max-width:100%;height:auto}a{cursor:pointer}'

/** サニタイズ済み HTML を sandbox iframe の srcdoc 用の完全な文書にする。CSP の meta を先頭に置く。 */
export function buildSrcdoc(sanitizedHtml: string, opts: { allowImages: boolean }): string {
  return (
    `<!doctype html><html${frameRootAttr()}><head>` +
    `<meta http-equiv="Content-Security-Policy" content="${buildCsp(opts.allowImages)}">` +
    `<meta charset="utf-8"><style>${BASE_STYLE}${frameScrollbarCss()}</style></head><body>${sanitizedHtml}</body></html>`
  )
}

/** システムブラウザ／メーラーで開いてよい URL か（http／https／mailto のみ。NF-S07）。 */
export function isOpenableUrl(url: string): boolean {
  try {
    return new URL(url.trim()).protocol in OPENABLE_PROTOCOLS
  } catch {
    return false
  }
}
