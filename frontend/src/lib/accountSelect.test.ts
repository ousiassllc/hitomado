import { describe, expect, test } from 'bun:test'
import { nextAccountIds } from './accountSelect'

describe('nextAccountIds', () => {
  test('通常クリックは単一選択。選択済みの単一を押すと全アカウントへ', () => {
    expect(nextAccountIds([], 'a', false)).toEqual(['a'])
    expect(nextAccountIds(['b'], 'a', false)).toEqual(['a'])
    expect(nextAccountIds(['a'], 'a', false)).toEqual([])
  })

  test('Ctrl/Cmd（スマホのチップは常に複数選択）は加える／外す', () => {
    expect(nextAccountIds(['a'], 'b', true)).toEqual(['a', 'b'])
    expect(nextAccountIds(['a', 'b'], 'a', true)).toEqual(['b'])
    expect(nextAccountIds(['a'], 'a', true)).toEqual([])
  })
})
