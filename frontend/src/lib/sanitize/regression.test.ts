import '../../../test/domPurifyShim'
import { describe, expect, test } from 'bun:test'
import { sanitizeDeclarations, sanitizeStyleSheet } from './css'
import { sanitizeEmailHtml } from './index'

const clean = (html: string, allowRemoteImages = false) =>
  sanitizeEmailHtml(html, { allowRemoteImages })

function parse(html: string): HTMLElement {
  const root = document.createElement('div')
  root.innerHTML = html
  return root
}

describe('レイアウト属性は URI 以外なので保たれる', () => {
  const html =
    '<table width="600" align="center" cellpadding="0" cellspacing="2" border="0" bgcolor="#eeeeee"><tbody><tr>' +
    '<td colspan="2" rowspan="1" valign="top" dir="rtl" lang="ja" height="40" nowrap>' +
    '<font color="red" size="3" face="Arial">x</font></td></tr></tbody></table>' +
    '<img src="data:image/png;base64,AAAA" width="10" height="20" alt="i">'

  test('table・td・font・img の属性が残る', () => {
    const root = parse(clean(html))
    const attr = (sel: string, name: string) => root.querySelector(sel)?.getAttribute(name)
    expect(attr('table', 'width')).toBe('600')
    expect(attr('table', 'align')).toBe('center')
    expect(attr('table', 'cellpadding')).toBe('0')
    expect(attr('table', 'cellspacing')).toBe('2')
    expect(attr('table', 'border')).toBe('0')
    expect(attr('table', 'bgcolor')).toBe('#eeeeee')
    expect(attr('td', 'colspan')).toBe('2')
    expect(attr('td', 'valign')).toBe('top')
    expect(attr('td', 'dir')).toBe('rtl')
    expect(attr('td', 'lang')).toBe('ja')
    expect(attr('td', 'height')).toBe('40')
    expect(attr('font', 'color')).toBe('red')
    expect(attr('font', 'size')).toBe('3')
    expect(attr('font', 'face')).toBe('Arial')
    expect(attr('img', 'width')).toBe('10')
    expect(attr('img', 'height')).toBe('20')
    expect(attr('img', 'src')).toBe('data:image/png;base64,AAAA')
  })

  test('画像許可時も同じ', () => {
    const root = parse(clean(html, true))
    expect(root.querySelector('table')?.getAttribute('width')).toBe('600')
    expect(root.querySelector('font')?.getAttribute('face')).toBe('Arial')
  })

  test('リンクの制限はそのまま（http/https/mailto 以外は href を失う）', () => {
    const root = parse(
      clean(
        '<a id="a" href="https://e.example/">a</a><a id="b" href="ftp://e.example/">b</a><a id="c" href="tel:1">c</a><a id="d" href="  JavaScript:alert(1)">d</a><a id="e" href="data:text/html,x">e</a>',
      ),
    )
    expect(root.querySelector('#a')?.getAttribute('href')).toBe('https://e.example/')
    for (const id of ['#b', '#c', '#d', '#e']) {
      expect(root.querySelector(id)?.hasAttribute('href')).toBe(false)
    }
  })

  test('area・map・usemap は除去される', () => {
    const out = clean(
      '<img src="data:image/png;base64,AAAA" usemap="#m"><map name="m"><area href="https://e.example/" shape="rect" coords="0,0,1,1"></map>',
    )
    expect(out).not.toMatch(/<map|<area|usemap/i)
  })
})

describe('閉じていない url( は宣言ごと除去する', () => {
  for (const allow of [false, true]) {
    test(`インライン style（画像許可=${allow}）`, () => {
      for (const css of [
        'background:url(https://evil.example/a.png',
        'background:url(data:image/png;base64,AAAA), url(https://evil.example/a.png',
        'background:url("https://evil.example/a.png',
        'background:url("https://evil.example/a.png" junk)',
        'background:url(   ',
        'background:URL(https://evil.example/a.png',
      ]) {
        const out = sanitizeDeclarations(`${css}; color: red`, { allowRemoteImages: allow })
        // url( を含む宣言は残らない（閉じ括弧が無いと後続の宣言も巻き込んで落ちることがある）
        expect(out).not.toMatch(/url\(/i)
        expect(out).not.toContain('evil.example')
        expect(out).not.toContain('background')
      }
    })

    test(`<style>（画像許可=${allow}）`, () => {
      const out = clean('<style>p{background:url(https://evil.example/a.png</style><p>x</p>', allow)
      expect(out).not.toContain('evil.example')
      const out2 = clean(
        '<style>p{background:url(data:image/png;base64,AAAA), url(https://evil.example/b.png}</style><p>x</p>',
        allow,
      )
      expect(out2).not.toContain('evil.example')
    })
  }

  test('閉じた url() は従来どおり（data: は残り、許可時のみ https: が残る）', () => {
    const data = 'background:url(data:image/png;base64,AAAA)'
    expect(sanitizeDeclarations(data, { allowRemoteImages: false })).toBe(data)
    const https = 'background:url("https://e.example/a.png")'
    expect(sanitizeDeclarations(https, { allowRemoteImages: false })).toBe('')
    expect(sanitizeDeclarations(https, { allowRemoteImages: true })).toBe(https)
    expect(
      sanitizeDeclarations('background:url(http://e.example/a.png)', { allowRemoteImages: true }),
    ).toBe('')
  })

  test('複数の url() の 1 つでも不許可なら宣言ごと除去する', () => {
    const css = 'background:url(data:image/png;base64,AAAA), url(https://e.example/a.png)'
    expect(sanitizeDeclarations(css, { allowRemoteImages: false })).toBe('')
    expect(sanitizeDeclarations(css, { allowRemoteImages: true })).toBe(css)
  })
})

describe('CSS サニタイズは入力長に対して線形（ReDoS 回帰）', () => {
  function timeOf(f: () => void): number {
    const t = performance.now()
    f()
    return performance.now() - t
  }

  const cases: Record<string, (n: number) => string> = {
    'url( + 空白': (n) => `background:url(${' '.repeat(n)}`,
    'url( + 空白 + x': (n) => `background:url(${' '.repeat(n)}x`,
    'url( の連続': (n) => `background:${'url('.repeat(n)}`,
    'url("a" の後ろに空白': (n) => `background:url("a"${' '.repeat(n)}x`,
    '閉じた url( の連続': (n) => `background:${'url(data:image/png;base64,A) '.repeat(n)}`,
  }

  for (const [name, make] of Object.entries(cases)) {
    test(name, () => {
      const small = make(2000)
      const large = make(20000)
      // ウォームアップ
      sanitizeDeclarations(small, { allowRemoteImages: true })
      const t1 = Math.max(
        timeOf(() => sanitizeDeclarations(small, { allowRemoteImages: true })),
        0.05,
      )
      const t2 = timeOf(() => sanitizeDeclarations(large, { allowRemoteImages: true }))
      // n を 10 倍にしても 100 倍未満（二次以上なら ~100 倍超）。絶対値の上限も緩く置く。
      expect(t2).toBeLessThan(t1 * 100)
      expect(t2).toBeLessThan(500)
    })
  }

  test('<style> 全体でも空白 20,000 個の url( が短時間で処理される', () => {
    const css = `p{background:url(${' '.repeat(20000)}}`
    expect(timeOf(() => sanitizeStyleSheet(css, { allowRemoteImages: true }))).toBeLessThan(500)
  })
})

describe('attr() を含む宣言は除去する', () => {
  test('url を attr() で組み立てる回避を許さない', () => {
    for (const allow of [false, true]) {
      const out = sanitizeDeclarations(
        'background: image-set(attr(data-u)); width: attr(data-w px); color: red',
        {
          allowRemoteImages: allow,
        },
      )
      expect(out).toBe('color: red')
      expect(sanitizeDeclarations('content: ATTR (data-x)', { allowRemoteImages: allow })).toBe('')
    }
  })
})
