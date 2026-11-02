import { describe, expect, test } from 'bun:test'
import { bindFrameDocument, MAX_HEIGHT } from './bodyFrame'

function frameDoc(html: string, height: number): Document {
  const doc = document.implementation.createHTMLDocument('')
  doc.body.innerHTML = html
  Object.defineProperty(doc.body, 'scrollHeight', { value: height, configurable: true })
  return doc
}

function fire(doc: Document, id: string, type: 'click' | 'auxclick', button = 0): MouseEvent {
  const ev = new MouseEvent(type, { bubbles: true, cancelable: true, button })
  doc.getElementById(id)?.dispatchEvent(ev)
  return ev
}

function setup(html: string, height = 321) {
  const heights: number[] = []
  const links: string[] = []
  const doc = frameDoc(html, height)
  const binding = bindFrameDocument(doc, {
    onHeight: (h) => heights.push(h),
    onLink: (u) => links.push(u),
  })
  return { doc, heights, links, binding }
}

describe('bindFrameDocument', () => {
  test('高さを通知し、許可スキームのリンクだけ onLink に渡し、ナビゲーションは常に止める', () => {
    const { doc, heights, links, binding } = setup(
      '<a id="ok" href="https://example.com/x"><span id="in">x</span></a><a id="mail" href="mailto:a@example.com">m</a><a id="js" href="javascript:alert(1)">j</a><a id="file" href="file:///etc/passwd">f</a><a id="rel" href="/x">r</a>',
    )
    expect(binding?.measured).toBe(true)
    expect(heights[0]).toBe(321)

    for (const id of ['in', 'mail', 'js', 'file', 'rel']) {
      expect(fire(doc, id, 'click').defaultPrevented).toBe(true)
    }
    expect(links).toEqual(['https://example.com/x', 'mailto:a@example.com'])

    binding?.release()
    fire(doc, 'ok', 'click')
    expect(links).toHaveLength(2)
  })

  test('中クリックは外部で開くが、右クリック（auxclick button=2）では何も起きない', () => {
    const { doc, links } = setup('<a id="a" href="https://example.com/">x</a>')
    const right = fire(doc, 'a', 'auxclick', 2)
    expect(right.defaultPrevented).toBe(false)
    expect(links).toEqual([])
    const middle = fire(doc, 'a', 'auxclick', 1)
    expect(middle.defaultPrevented).toBe(true)
    expect(links).toEqual(['https://example.com/'])
  })

  test('area[href] のクリックも捕捉する（許可スキームのみ）', () => {
    const { doc, links } = setup(
      '<map><area id="ok" href="https://example.com/m"><area id="bad" href="javascript:alert(1)"></map>',
    )
    expect(fire(doc, 'ok', 'click').defaultPrevented).toBe(true)
    expect(fire(doc, 'bad', 'click').defaultPrevented).toBe(true)
    expect(links).toEqual(['https://example.com/m'])
  })

  test('高さが測れなくてもリンク捕捉は取り付ける（固定高へフォールバック）', () => {
    const { doc, heights, links, binding } = setup('<a id="a" href="https://example.com/">x</a>', 0)
    expect(binding).toBeDefined()
    expect(binding?.measured).toBe(false)
    expect(heights).toEqual([])
    expect(fire(doc, 'a', 'click').defaultPrevented).toBe(true)
    expect(links).toEqual(['https://example.com/'])
  })

  test('高さ計測が例外を投げてもリンク捕捉は動く', () => {
    const doc = frameDoc('<a id="a" href="https://example.com/">x</a>', 100)
    Object.defineProperty(doc.body, 'scrollHeight', {
      get() {
        throw new Error('boom')
      },
      configurable: true,
    })
    const links: string[] = []
    const binding = bindFrameDocument(doc, { onHeight() {}, onLink: (u) => links.push(u) })
    expect(binding?.measured).toBe(false)
    fire(doc, 'a', 'click')
    expect(links).toEqual(['https://example.com/'])
  })

  test('文書に触れない場合は undefined', () => {
    expect(bindFrameDocument(undefined, { onHeight() {}, onLink() {} })).toBeUndefined()
    const throwing = {
      get documentElement(): never {
        throw new Error('SecurityError')
      },
    } as unknown as Document
    expect(bindFrameDocument(throwing, { onHeight() {}, onLink() {} })).toBeUndefined()
  })
})

describe('高さの上限', () => {
  test('上限以内はそのままの高さで、スクロールバーを出さない（overflow: hidden）', () => {
    const { doc, heights } = setup('<p>x</p>', MAX_HEIGHT)
    expect(heights).toEqual([MAX_HEIGHT])
    expect(doc.documentElement.style.overflow).toBe('hidden')
  })

  test('上限を超えたら上限で止め、iframe 内部のスクロール（overflow: auto）にする', () => {
    const { doc, heights, binding } = setup('<p>x</p>', 50000)
    expect(heights).toEqual([MAX_HEIGHT])
    expect(doc.documentElement.style.overflow).toBe('auto')
    expect(binding?.measured).toBe(true)
  })

  test('内容が伸び続けても（min-height:100vh 等）通知は上限を超えない。縮んだら hidden に戻る', () => {
    const { doc, heights } = setup('<a id="a" href="https://example.com/">x</a>', 9000)
    for (const px of [20000, 1e9, 4000]) {
      Object.defineProperty(doc.body, 'scrollHeight', { value: px, configurable: true })
      doc.dispatchEvent(new Event('load'))
    }
    expect(Math.max(...heights)).toBe(MAX_HEIGHT)
    expect(heights.at(-1)).toBe(4000)
    expect(doc.documentElement.style.overflow).toBe('hidden')
  })
})
