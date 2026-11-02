import '../../../test/domPurifyShim'
import { describe, expect, test } from 'bun:test'
import { buildCsp, buildSrcdoc, isOpenableUrl, sanitizeEmailHtml } from './index'

const clean = (html: string, allowRemoteImages = false) =>
  sanitizeEmailHtml(html, { allowRemoteImages })

/** サニタイズ結果を DOM にして、実行可能・外部参照の要素や属性が残っていないことを確かめる。 */
function parse(html: string): HTMLElement {
  const root = document.createElement('div')
  root.innerHTML = html
  return root
}

describe('sanitizeEmailHtml: XSS ペイロード', () => {
  const payloads: Record<string, string> = {
    script: '<p>a</p><script>alert(1)</script>',
    onerror: '<img src="data:image/png;base64,AAAA" onerror="alert(1)">',
    onclick: '<a href="https://example.com" onclick="alert(1)">x</a>',
    jsHref: '<a href="javascript:alert(1)">x</a>',
    jsHrefObfuscated: '<a href="jav&#x09;ascript:alert(1)">x</a>',
    svgOnload: '<svg onload="alert(1)"><circle r="1"></circle></svg>',
    svgScript: '<svg><script>alert(1)</script></svg>',
    iframe: '<iframe src="https://evil.example"></iframe>',
    iframeSrcdoc: '<iframe srcdoc="<script>alert(1)</script>"></iframe>',
    object: '<object data="https://evil.example/x.swf"></object>',
    embed: '<embed src="https://evil.example/x.swf">',
    form: '<form action="https://evil.example"><input name="pw"><button>go</button></form>',
    link: '<link rel="stylesheet" href="https://evil.example/x.css">',
    base: '<base href="https://evil.example/"><a href="x">x</a>',
    metaRefresh: '<meta http-equiv="refresh" content="0;url=https://evil.example">',
    fileHref: '<a href="file:///etc/passwd">x</a>',
    dataHref: '<a href="data:text/html,<script>alert(1)</script>">x</a>',
    cssExpression: '<div style="width: expression(alert(1))">x</div>',
    cssJsUrl: '<div style="background: url(javascript:alert(1))">x</div>',
    cssEscapedUrl: '<div style="background: \\75rl(https://evil.example/x.png)">x</div>',
    styleImport: '<style>@import url("https://evil.example/x.css"); p { color: red }</style>',
    styleBehavior: '<style>p { behavior: url(x.htc) }</style>',
    styleBreakout: '<style>p::after { content: "</style><script>alert(1)</script>" }</style>',
    mathml: '<math><mi xlink:href="javascript:alert(1)">x</mi></math>',
    video: '<video poster="https://evil.example/p.png" src="https://evil.example/v.mp4"></video>',
    clobber: '<img name="body" src="data:image/png;base64,AAAA"><form id="x"></form>',
  }

  for (const [name, payload] of Object.entries(payloads)) {
    for (const allow of [false, true]) {
      test(`${name}（画像許可=${allow}）`, () => {
        const out = clean(payload, allow)
        const root = parse(out)
        const forbidden =
          'script,iframe,object,embed,form,input,button,link,base,meta,video,svg,math'
        expect(root.querySelector(forbidden)).toBeNull()
        for (const el of root.querySelectorAll('*')) {
          for (const attr of el.getAttributeNames()) {
            expect(attr.startsWith('on')).toBe(false)
          }
          const href = el.getAttribute('href')
          if (href !== null) expect(href).toMatch(/^(?:https?:|mailto:)/i)
        }
        expect(out).not.toMatch(/javascript:/i)
        expect(out).not.toMatch(/expression\s*\(/i)
        expect(out).not.toMatch(/<\/style><script/i)
        expect(out).not.toMatch(/file:/i)
        expect(out).not.toMatch(/behavior/i)
        expect(out).not.toMatch(/@import/i)
      })
    }
  }

  test('通常の書式・リンク・表は残る', () => {
    const out = clean(
      '<table style="width:100%"><tr><td bgcolor="#eee"><b>太字</b> <a href="https://example.com/a">link</a></td></tr></table>',
    )
    const root = parse(out)
    expect(root.querySelector('table')?.getAttribute('style')).toContain('width:100%')
    expect(root.querySelector('b')?.textContent).toBe('太字')
    const a = root.querySelector('a')
    expect(a?.getAttribute('href')).toBe('https://example.com/a')
    expect(a?.getAttribute('rel')).toBe('noopener noreferrer')
    expect(a?.hasAttribute('target')).toBe(false)
  })

  test('mailto リンクは残り、tel・相対・フラグメントは href を失う', () => {
    const root = parse(
      clean(
        '<a id="m" href="mailto:a@example.com">m</a><a id="t" href="tel:123">t</a><a id="r" href="/x">r</a><a id="f" href="#top">f</a>',
      ),
    )
    expect(root.querySelector('#m')?.getAttribute('href')).toBe('mailto:a@example.com')
    for (const id of ['#t', '#r', '#f'])
      expect(root.querySelector(id)?.hasAttribute('href')).toBe(false)
  })

  test('<style> の通常ルールと @media は残り、@import・@font-face は除去される', () => {
    const out = clean(
      '<style>@import "https://e.example/a.css"; @font-face { font-family: x; src: url(https://e.example/f.woff) } p { color: red; background: url(https://e.example/b.png) } @media (max-width: 600px) { .a { width: 100% } }</style><p>x</p>',
    )
    expect(out).toContain('color: red')
    expect(out).toContain('@media (max-width: 600px)')
    expect(out).toContain('.a{width: 100%}')
    expect(out).not.toContain('@import')
    expect(out).not.toContain('font-face')
    expect(out).not.toContain('b.png')
  })

  test('先頭の <style> が失われない', () => {
    expect(clean('<style>p{color:red}</style><p>x</p>')).toContain('p{color:red}')
  })
})

describe('sanitizeEmailHtml: インラインスタイルの url()', () => {
  test('url() を含む宣言のみ除去し、他の宣言は残す', () => {
    const root = parse(
      clean(
        '<div style="color: red; background: url(https://e.example/a.png) no-repeat; margin: 0">x</div>',
      ),
    )
    const style = root.querySelector('div')?.getAttribute('style') ?? ''
    expect(style).toContain('color: red')
    expect(style).toContain('margin: 0')
    expect(style).not.toContain('url(')
  })

  test('url() のみのスタイルは style 属性ごと消える', () => {
    const root = parse(
      clean('<div style="background-image:url(&quot;https://e.example/a.png&quot;)">x</div>'),
    )
    expect(root.querySelector('div')?.hasAttribute('style')).toBe(false)
  })

  test('data: 画像の url() は常に残る', () => {
    const root = parse(clean('<div style="background:url(data:image/png;base64,AAAA)">x</div>'))
    expect(root.querySelector('div')?.getAttribute('style')).toContain('data:image/png;base64,AAAA')
  })

  test('画像許可時は https: の url() が復元され、http:・相対・javascript: は残らない', () => {
    const html =
      '<div id="a" style="background:url(https://e.example/a.png)">x</div>' +
      '<div id="b" style="background:url(http://e.example/a.png)">x</div>' +
      '<div id="c" style="background:url(/a.png)">x</div>' +
      '<div id="d" style="background:url(javascript:alert(1))">x</div>'
    const root = parse(clean(html, true))
    expect(root.querySelector('#a')?.getAttribute('style')).toContain('https://e.example/a.png')
    for (const id of ['#b', '#c', '#d'])
      expect(root.querySelector(id)?.hasAttribute('style')).toBe(false)
  })

  test('image-set・expression を含む宣言は許可時でも除去される', () => {
    const root = parse(
      clean(
        '<div style="background: image-set(url(https://e.example/a.png) 1x); color: blue">x</div>',
        true,
      ),
    )
    expect(root.querySelector('div')?.getAttribute('style')).toBe('color: blue')
  })
})

describe('sanitizeEmailHtml: リモート画像ブロック（NF-S05）', () => {
  const remote =
    '<img id="r" src="https://e.example/a.png" srcset="https://e.example/a2.png 2x" alt="remote">' +
    '<img id="h" src="http://e.example/a.png">' +
    '<img id="d" src="data:image/png;base64,AAAA" alt="inline">' +
    '<img id="rel" src="//e.example/a.png">' +
    '<img id="cid" src="cid:abc">' +
    '<table background="https://e.example/bg.png"><tr><td>x</td></tr></table>'

  test('既定: data: 以外の src・srcset・background が除去される', () => {
    const root = parse(clean(remote))
    expect(root.querySelector('#r')?.hasAttribute('src')).toBe(false)
    expect(root.querySelector('#r')?.hasAttribute('srcset')).toBe(false)
    expect(root.querySelector('#r')?.getAttribute('alt')).toBe('remote')
    for (const id of ['#h', '#rel', '#cid'])
      expect(root.querySelector(id)?.hasAttribute('src')).toBe(false)
    expect(root.querySelector('#d')?.getAttribute('src')).toBe('data:image/png;base64,AAAA')
    expect(root.querySelector('table')?.hasAttribute('background')).toBe(false)
  })

  test('許可時: https: の src・background が復元される（元 HTML の再サニタイズ）', () => {
    const root = parse(clean(remote, true))
    expect(root.querySelector('#r')?.getAttribute('src')).toBe('https://e.example/a.png')
    expect(root.querySelector('#r')?.hasAttribute('srcset')).toBe(false)
    expect(root.querySelector('table')?.getAttribute('background')).toBe('https://e.example/bg.png')
    // http: ・プロトコル相対・cid: は許可時でも復元しない
    for (const id of ['#h', '#rel', '#cid'])
      expect(root.querySelector(id)?.hasAttribute('src')).toBe(false)
  })

  test('許可の有無が次の呼び出しに漏れない', () => {
    clean(remote, true)
    expect(parse(clean(remote)).querySelector('#r')?.hasAttribute('src')).toBe(false)
  })

  test('img の data:text/html は残らない', () => {
    const root = parse(clean('<img src="data:text/html;base64,AAAA">'))
    expect(root.querySelector('img')?.hasAttribute('src')).toBe(false)
  })
})

describe('buildSrcdoc / buildCsp', () => {
  test('CSP は画像ブロック時 data: のみ、許可時のみ https: を追加', () => {
    expect(buildCsp(false)).toBe("default-src 'none'; img-src data:; style-src 'unsafe-inline'")
    expect(buildCsp(true)).toBe(
      "default-src 'none'; img-src data: https:; style-src 'unsafe-inline'",
    )
  })

  test('CSP の meta が head の先頭にある', () => {
    const doc = buildSrcdoc('<p>x</p>', { allowImages: false })
    expect(
      doc.startsWith('<!doctype html><html><head><meta http-equiv="Content-Security-Policy"'),
    ).toBe(true)
    expect(doc).toContain("img-src data:; style-src 'unsafe-inline'")
    expect(doc).not.toContain('https:')
    expect(doc).toContain('<body><p>x</p></body>')
    expect(buildSrcdoc('', { allowImages: true })).toContain('img-src data: https:;')
  })
})

describe('isOpenableUrl', () => {
  test('http／https／mailto のみ true', () => {
    for (const u of [
      'http://a.example',
      'https://a.example/x?y=1',
      'mailto:a@example.com',
      ' https://a.example ',
    ]) {
      expect(isOpenableUrl(u)).toBe(true)
    }
  })

  test('それ以外は false', () => {
    for (const u of [
      'javascript:alert(1)',
      'JaVaScRiPt:alert(1)',
      'file:///etc/passwd',
      'data:text/html,x',
      'tel:123',
      'ftp://a.example',
      'about:blank',
      '/relative',
      '#frag',
      '',
      'constructor:x',
    ]) {
      expect(isOpenableUrl(u)).toBe(false)
    }
  })
})
