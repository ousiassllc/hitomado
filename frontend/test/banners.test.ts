import { describe, expect, test } from 'bun:test'
import { deriveBanners, selectBanners } from '../src/lib/banners'
import type { Banner, BannerKind } from '../src/stores/ui'

const b = (kind: BannerKind, key: string = kind): Banner => ({ key, kind, message: key })

describe('selectBanners', () => {
  test('優先度順（オフライン → 要再接続 → 更新あり → 同期エラー）', () => {
    const r = selectBanners([b('sync_error'), b('update'), b('needs_reauth', 'r1'), b('offline')], 10)
    expect(r.map((x) => x.kind)).toEqual(['offline', 'needs_reauth', 'update', 'sync_error'])
  })

  test('最大 2 件', () => {
    const r = selectBanners([b('sync_error'), b('update'), b('offline')])
    expect(r.map((x) => x.kind)).toEqual(['offline', 'update'])
  })

  test('同じ優先度は入力順、key 重複は 1 件にまとめる', () => {
    const r = selectBanners([b('needs_reauth', 'r2'), b('needs_reauth', 'r1'), b('needs_reauth', 'r2')], 10)
    expect(r.map((x) => x.key)).toEqual(['r2', 'r1'])
  })

  test('空', () => {
    expect(selectBanners([])).toEqual([])
  })
})

describe('deriveBanners', () => {
  const base = {
    online: true,
    accounts: [
      { id: 'a1', displayName: '会社A' },
      { id: 'a2', displayName: '会社B' },
    ],
    statuses: {},
    statusMessages: {},
    extra: [],
    reconnect: () => {},
  }

  test('正常時は何も出さない', () => {
    expect(deriveBanners(base)).toEqual([])
  })

  test('オフライン・要再接続・同期エラー・uiStore のバナーをまとめる', () => {
    const reconnected: string[] = []
    const r = deriveBanners({
      ...base,
      online: false,
      statuses: { a1: 'needs_reauth', a2: 'error' },
      statusMessages: { a2: '同期できません' },
      extra: [b('update')],
      reconnect: (id) => reconnected.push(id),
    })
    expect(r.map((x) => x.key)).toEqual(['offline', 'reauth:a1', 'sync_error:a2', 'update'])
    expect(r[1].message).toContain('会社A')
    expect(r[2].message).toBe('同期できません')
    r[1].action?.run()
    expect(reconnected).toEqual(['a1'])
    // 優先度順の最大 2 件はオフライン＋要再接続
    expect(selectBanners(r).map((x) => x.kind)).toEqual(['offline', 'needs_reauth'])
  })
})
