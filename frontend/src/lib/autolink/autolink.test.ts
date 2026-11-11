import { describe, expect, test } from 'bun:test'
import { bindFrameDocument } from '../../components/thread/bodyFrame'
import { sanitizeEmailHtml } from '../sanitize'
import { autolinkDom, findLinks, splitLinks } from '.'

const hrefs = (s: string): string[] => findLinks(s).map((l) => l.href)
const texts = (s: string): string[] => findLinks(s).map((l) => s.slice(l.start, l.end))

describe('findLinks', () => {
  test('http・https・www.・メールアドレスを見つけ、www. は https、メールは mailto にする', () => {
    expect(
      hrefs('a http://a.example/x b https://b.example c www.c.example/p d user@d.example'),
    ).toEqual([
      'http://a.example/x',
      'https://b.example',
      'https://www.c.example/p',
      'mailto:user@d.example',
    ])
  })

  test('末尾の句読点・閉じかっこを巻き込まない（URL 内の対のかっこは残す）', () => {
    expect(texts('見て https://e.example/a.')).toEqual(['https://e.example/a'])
    expect(texts('(https://e.example/a)')).toEqual(['https://e.example/a'])
    expect(texts('https://e.example/a_(b), https://e.example/c!?')).toEqual([
      'https://e.example/a_(b)',
      'https://e.example/c',
    ])
    expect(texts('[https://e.example/a]; <https://e.example/b>')).toEqual([
      'https://e.example/a',
      'https://e.example/b',
    ])
  })

  test('日本語の文字・全角の句読点・かっこを巻き込まない', () => {
    expect(texts('詳細はhttps://e.example/a?x=1です。')).toEqual(['https://e.example/a?x=1'])
    expect(texts('「https://e.example/a」を見て、https://e.example/b。')).toEqual([
      'https://e.example/a',
      'https://e.example/b',
    ])
    expect(texts('連絡先は a@e.example（至急）')).toEqual(['a@e.example'])
  })

  test('javascript:・file:・data:・ホストの無い殻・語の一部の www. は候補にしない', () => {
    expect(
      hrefs(
        'javascript:alert(1) file:///etc/passwd data:text/html,x https:// https://. ftp://e.example awww.e.example',
      ),
    ).toEqual([])
    expect(hrefs('x@y')).toEqual([])
  })

  test('https://user@host の @ をメールにせず、URL 内のメールアドレスも URL の一部のまま', () => {
    expect(hrefs('https://u@e.example/p?mail=a@b.example')).toEqual([
      'https://u@e.example/p?mail=a@b.example',
    ])
  })

  test('splitLinks は連結すると元の文字列に戻る', () => {
    const s = '前 https://e.example/a 中 u@e.example 後'
    expect(
      splitLinks(s)
        .map((x) => x.text)
        .join(''),
    ).toBe(s)
    expect(splitLinks(s).filter((x) => x.href)).toHaveLength(2)
    expect(splitLinks('')).toEqual([])
  })
})

describe('autolinkDom / sanitizeEmailHtml({ autolink })', () => {
  function body(html: string): Element {
    const doc = document.implementation.createHTMLDocument('')
    doc.body.innerHTML = html
    return doc.body
  }

  test('a・script・style の中は変えず、素の URL だけ a にする（引用・署名の中も）', () => {
    const root = body(
      '<p>https://p.example</p><blockquote>> https://q.example</blockquote><div>-- <br>https://sig.example</div><a href="https://k.example">https://k.example</a><a>https://nohref.example</a>',
    )
    autolinkDom(root)
    const links = Array.from(root.querySelectorAll('a')).map((a) => a.getAttribute('href'))
    expect(links).toEqual([
      'https://p.example',
      'https://q.example',
      'https://sig.example',
      'https://k.example',
      null,
    ])
    expect(root.querySelector('blockquote')?.textContent).toBe('> https://q.example')
    expect(root.querySelectorAll('a a')).toHaveLength(0)
  })

  test('autolink は既定でオフ。オンでも危険なスキームは a にならず、リンクは rel のみで target を持たない', () => {
    const html = '<p>https://p.example javascript:alert(1)</p><a href="javascript:alert(2)">x</a>'
    expect(sanitizeEmailHtml(html)).not.toContain('https://p.example</a>')
    const out = sanitizeEmailHtml(html, { autolink: true })
    expect(out).toContain(
      '<a href="https://p.example" rel="noopener noreferrer">https://p.example</a>',
    )
    expect(out).not.toContain('href="javascript')
    expect(out).not.toContain('target=')
    expect(out).toContain('javascript:alert(1)')
  })

  test('エスケープされたタグ風の文字列は a の中でも要素にならない', () => {
    const out = sanitizeEmailHtml('<p>https://p.example/&lt;img src=x onerror=alert(1)&gt;</p>', {
      autolink: true,
    })
    expect(out).not.toContain('<img')
  })
})

describe('自動リンクのクリック（bindFrameDocument）', () => {
  test('素の URL のリンクは onLink に渡り、ナビゲーションは止まる。javascript: の文字列は何も起きない', () => {
    const doc = document.implementation.createHTMLDocument('')
    doc.body.innerHTML = sanitizeEmailHtml(
      '<p id="p"><span id="u">https://p.example/x</span> <i id="j">javascript:alert(1)</i></p>',
      { autolink: true },
    )
    const links: string[] = []
    const binding = bindFrameDocument(doc, { onHeight: () => {}, onLink: (u) => links.push(u) })
    const click = (id: string): boolean => {
      const ev = new MouseEvent('click', { bubbles: true, cancelable: true })
      doc.getElementById(id)?.dispatchEvent(ev)
      return ev.defaultPrevented
    }
    expect(doc.querySelectorAll('a')).toHaveLength(1)
    const a = doc.querySelector('#u a')
    const ev = new MouseEvent('click', { bubbles: true, cancelable: true })
    a?.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(true)
    expect(links).toEqual(['https://p.example/x'])
    expect(click('j')).toBe(false)
    binding?.release()
  })
})
