import { describe, expect, test } from 'bun:test'
import { cleanRichHtml, normalizeLinkUrl } from './richtext'
import {
  applySignature,
  buildSignatureBlock,
  ensureLeadingParagraph,
  findSignature,
  hasUserBody,
  normalizeSignature,
  replaceSignature,
  replaceUserBody,
  signatureContent,
} from './signature'

const QUOTE = '<blockquote data-hitomado-quote><p>引用</p></blockquote>'

describe('buildSignatureBlock', () => {
  test('Go の signature.Block と同じ形式（-- 区切り）', () => {
    expect(buildSignatureBlock('a1', '<p>山田</p>')).toBe(
      '<div data-hitomado-signature data-account-id="a1">-- <br><p>山田</p></div>',
    )
  })
  test('署名が空・空白のみなら空文字', () => {
    expect(buildSignatureBlock('a1', '')).toBe('')
    expect(buildSignatureBlock('a1', '  <p> </p> ')).toBe('')
  })
  test('許可外のタグ・属性・危険な href を除く', () => {
    const c = signatureContent(
      '<p onclick="x()">a<script>alert(1)</script><a href="javascript:x()">b</a></p>',
    )
    expect(c).not.toContain('script')
    expect(c).not.toContain('onclick')
    expect(c).not.toContain('javascript')
  })
})

describe('findSignature', () => {
  test('ブロックのアカウントと内容を返す', () => {
    const html = `<p>本文</p>${buildSignatureBlock('a1', 'sig')}${QUOTE}`
    expect(findSignature(html)).toEqual({ accountId: 'a1', content: '-- <br>sig' })
  })
  test('無ければ undefined', () => {
    expect(findSignature('<p>本文</p>')).toBeUndefined()
  })
})

describe('applySignature', () => {
  test('引用の直前に挿入する', () => {
    const out = applySignature(`<p>本文</p>${QUOTE}`, 'a1', 'sig')
    expect(out.indexOf('data-hitomado-signature')).toBeLessThan(out.indexOf('data-hitomado-quote'))
    expect(out.indexOf('本文')).toBeLessThan(out.indexOf('data-hitomado-signature'))
  })
  test('引用が無ければ末尾に挿入する', () => {
    const out = applySignature('<p>本文</p>', 'a1', 'sig')
    expect(out.endsWith('</div>')).toBe(true)
    expect(findSignature(out)?.accountId).toBe('a1')
  })
  test('既存ブロックは差し替える（複製しない）', () => {
    const first = applySignature('<p>本文</p>', 'a1', 'one')
    const out = applySignature(first, 'a2', 'two')
    expect(out.match(/data-hitomado-signature/g)).toHaveLength(1)
    expect(findSignature(out)).toEqual({ accountId: 'a2', content: '-- <br>two' })
  })
  test('署名が空ならブロックを削除する', () => {
    const first = applySignature(`<p>本文</p>${QUOTE}`, 'a1', 'one')
    const out = applySignature(first, 'a2', '')
    expect(out).not.toContain('data-hitomado-signature')
    expect(out).toContain('data-hitomado-quote')
  })
  test('引用が入れ子の要素内にあっても最上位の手前に挿入する', () => {
    const out = applySignature(`<p>本文</p><div>${QUOTE}</div>`, 'a1', 'sig')
    expect(out.indexOf('data-hitomado-signature')).toBeLessThan(out.indexOf('<div><blockquote'))
  })
})

describe('replaceSignature', () => {
  const inserted = signatureContent('<p>山田</p>')
  const base = `<p>本文</p>${buildSignatureBlock('a1', '<p>山田</p>')}${QUOTE}`

  test('未編集なら切替先の署名に差し替える', () => {
    const r = replaceSignature(base, { fromSig: inserted, toSig: '<p>佐藤</p>', toAccountId: 'a2' })
    expect(r.status).toBe('replaced')
    expect(findSignature(r.html)?.accountId).toBe('a2')
    expect(r.html).toContain('佐藤')
    expect(r.html).not.toContain('山田')
    expect(r.html).toContain('data-hitomado-quote')
  })

  test('エディタ再シリアライズ後（p でラップ・斜体除去）でも未編集と判定する', () => {
    const fromSig = signatureContent('<p>山田 <em>太郎</em> <strong>太</strong></p>')
    const roundTripped = `<p>本文</p><div data-hitomado-signature="" data-account-id="a1"><p>-- <br>山田 太郎 <strong>太</strong></p></div>`
    const r = replaceSignature(roundTripped, { fromSig, toSig: 'x', toAccountId: 'a2' })
    expect(r.status).toBe('replaced')
  })

  test('編集済みなら変更せず edited を返す', () => {
    const edited = base.replace('山田', '山田（編集）')
    const r = replaceSignature(edited, { fromSig: inserted, toSig: 'sig', toAccountId: 'a2' })
    expect(r.status).toBe('edited')
    expect(r.html).toBe(edited)
  })

  test('署名なしアカウントへの切替はブロックを削除する', () => {
    const r = replaceSignature(base, { fromSig: inserted, toSig: '', toAccountId: 'a2' })
    expect(r.status).toBe('replaced')
    expect(r.html).not.toContain('data-hitomado-signature')
  })

  test('ブロックが無ければ absent で html はそのまま', () => {
    const r = replaceSignature('<p>本文</p>', {
      fromSig: inserted,
      toSig: 'sig',
      toAccountId: 'a2',
    })
    expect(r).toEqual({ html: '<p>本文</p>', status: 'absent' })
  })

  test('直前の署名が不明（fromSig なし）でブロックに内容があれば edited', () => {
    const r = replaceSignature(base, { fromSig: undefined, toSig: 'sig', toAccountId: 'a2' })
    expect(r.status).toBe('edited')
  })

  test('利用者が内容を空にしたブロックは差し替え可', () => {
    const emptied = '<p>本文</p><div data-hitomado-signature data-account-id="a1"><p></p></div>'
    const r = replaceSignature(emptied, { fromSig: inserted, toSig: 'sig', toAccountId: 'a2' })
    expect(r.status).toBe('replaced')
  })
})

describe('normalizeSignature', () => {
  test('段落と br・行末空白の違いを吸収する', () => {
    expect(normalizeSignature('-- <br>a<br>b')).toBe(normalizeSignature('<p>-- <br>a</p><p>b</p>'))
  })
  test('文言の違いは区別する', () => {
    expect(normalizeSignature('<p>a</p>')).not.toBe(normalizeSignature('<p>b</p>'))
  })
  test('リンク先の違いは区別する', () => {
    expect(normalizeSignature('<a href="https://a.example">x</a>')).not.toBe(
      normalizeSignature('<a href="https://b.example">x</a>'),
    )
  })
})

describe('本文の置換', () => {
  const html = `<p>下書き</p>${buildSignatureBlock('a1', 'sig')}${QUOTE}`
  test('hasUserBody は署名・引用を数えない', () => {
    expect(hasUserBody(html)).toBe(true)
    expect(hasUserBody(`<p></p>${buildSignatureBlock('a1', 'sig')}${QUOTE}`)).toBe(false)
    expect(hasUserBody(`${buildSignatureBlock('a1', 'sig')}${QUOTE}`)).toBe(false)
  })
  test('replaceUserBody は署名・引用を保持する', () => {
    const out = replaceUserBody(html, '<p>AI の本文</p>')
    expect(out.startsWith('<p>AI の本文</p>')).toBe(true)
    expect(out).not.toContain('下書き')
    expect(findSignature(out)?.accountId).toBe('a1')
    expect(out).toContain('data-hitomado-quote')
  })
  test('署名・引用が無い場合は全体を置き換える', () => {
    expect(replaceUserBody('<p>a</p>', '<p>b</p>')).toBe('<p>b</p>')
  })
  test('ensureLeadingParagraph は署名が先頭のときだけ空段落を足す', () => {
    const sig = buildSignatureBlock('a1', 'sig')
    expect(ensureLeadingParagraph(sig)).toBe(`<p></p>${sig}`)
    expect(ensureLeadingParagraph(QUOTE)).toBe(`<p></p>${QUOTE}`)
    expect(ensureLeadingParagraph(`<p>a</p>${sig}`)).toBe(`<p>a</p>${sig}`)
  })
})

describe('cleanRichHtml', () => {
  test('許可タグのみ残し、ブロックは段落に、ol は箇条書きにする', () => {
    const out = cleanRichHtml(
      '<h1 style="x">見出し</h1><div>本文<b>太</b><i>斜</i></div><ol><li>一</li></ol><img src=x onerror=alert(1)>',
    )
    expect(out).toContain('<p>見出し</p>')
    expect(out).toContain('<p>本文<b>太</b>斜</p>')
    expect(out).toContain('<ul><li>一</li></ul>')
    expect(out).not.toContain('img')
    expect(out).not.toContain('style')
  })
  test('署名・引用の属性は貼り付けで持ち込まない', () => {
    const out = cleanRichHtml(`<div data-hitomado-signature data-account-id="a1">x</div>`)
    expect(out).not.toContain('data-')
  })
  test('http／https／mailto 以外のリンクを除く', () => {
    const out = cleanRichHtml(
      '<a href="javascript:alert(1)">a</a><a href="https://x.example">b</a>',
    )
    expect(out).not.toContain('javascript')
    expect(out).toContain('href="https://x.example"')
  })
})

describe('normalizeLinkUrl', () => {
  test('スキームの扱い', () => {
    expect(normalizeLinkUrl('https://a.example')).toBe('https://a.example')
    expect(normalizeLinkUrl('mailto:a@b.example')).toBe('mailto:a@b.example')
    expect(normalizeLinkUrl('a.example/x')).toBe('https://a.example/x')
    expect(normalizeLinkUrl('a@b.example')).toBe('mailto:a@b.example')
    expect(normalizeLinkUrl('javascript:alert(1)')).toBeUndefined()
    expect(normalizeLinkUrl('data:text/html,x')).toBeUndefined()
    expect(normalizeLinkUrl('//evil.example')).toBeUndefined()
    expect(normalizeLinkUrl(' ')).toBeUndefined()
  })
})
