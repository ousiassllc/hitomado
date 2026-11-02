import { describe, expect, test } from 'bun:test'
import { addressLabel, isLikelyEmail, mergeAddresses, parseAddresses } from './recipients'

describe('parseAddresses', () => {
  test('カンマ・セミコロン・改行・空白区切りを分割する', () => {
    expect(parseAddresses('a@x.example, b@x.example;c@x.example\nd@x.example e@x.example')).toEqual(
      [
        { email: 'a@x.example' },
        { email: 'b@x.example' },
        { email: 'c@x.example' },
        { email: 'd@x.example' },
        { email: 'e@x.example' },
      ],
    )
  })
  test('表示名付き（引用符内のカンマを含む）', () => {
    expect(parseAddresses('"Yamada, Taro" <Taro@X.example>, Hanako <h@x.example>')).toEqual([
      { name: 'Yamada, Taro', email: 'taro@x.example' },
      { name: 'Hanako', email: 'h@x.example' },
    ])
  })
  test('山括弧のみ・空要素', () => {
    expect(parseAddresses(' <a@x.example> ,, ')).toEqual([{ email: 'a@x.example' }])
    expect(parseAddresses('')).toEqual([])
  })
})

describe('mergeAddresses', () => {
  test('大文字小文字を区別せず重複を除く', () => {
    const merged = mergeAddresses(
      [{ email: 'a@x.example' }],
      [{ email: 'A@x.example' }, { email: 'b@x.example' }],
    )
    expect(merged.map((a) => a.email)).toEqual(['a@x.example', 'b@x.example'])
  })
})

describe('isLikelyEmail / addressLabel', () => {
  test('形式', () => {
    expect(isLikelyEmail('a@x.example')).toBe(true)
    expect(isLikelyEmail('a@')).toBe(false)
    expect(isLikelyEmail('a b@x')).toBe(false)
  })
  test('ラベル', () => {
    expect(addressLabel({ name: '山田', email: 'y@x.example' })).toBe('山田 <y@x.example>')
    expect(addressLabel({ email: 'y@x.example' })).toBe('y@x.example')
  })
})
