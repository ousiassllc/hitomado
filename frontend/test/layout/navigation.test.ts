import { describe, expect, test } from 'bun:test'
import { backActionOf, type NavState, screenOf } from '../../src/lib/navigation'

const base: NavState = { overlayOpen: false, menuOpen: false, threadOpen: false, searching: false }

describe('screenOf', () => {
  test('一覧 → スレッド → 返信・転送 の順に進む', () => {
    expect(screenOf({ threadOpen: false })).toBe('list')
    expect(screenOf({ threadOpen: true })).toBe('thread')
    expect(screenOf({ threadOpen: true, composeMode: 'reply' })).toBe('reply')
    expect(screenOf({ threadOpen: true, composeMode: 'reply_all' })).toBe('reply')
    expect(screenOf({ threadOpen: true, composeMode: 'forward' })).toBe('reply')
  })

  test('新規作成はスレッドを開いていなくても作成画面', () => {
    expect(screenOf({ threadOpen: false, composeMode: 'new' })).toBe('compose')
    expect(screenOf({ threadOpen: true, composeMode: 'new' })).toBe('compose')
  })
})

describe('backActionOf', () => {
  test('返信 → スレッド、スレッド → 一覧、一覧（ルート）は戻る先なし', () => {
    expect(backActionOf({ ...base, threadOpen: true, composeMode: 'reply' })).toBe('closeCompose')
    expect(backActionOf({ ...base, threadOpen: true })).toBe('closeThread')
    expect(backActionOf(base)).toBeUndefined()
  })

  test('新規作成は閉じて元の画面へ', () => {
    expect(backActionOf({ ...base, composeMode: 'new' })).toBe('closeCompose')
  })

  test('検索結果の一覧は、戻るで受信箱へ戻る。その次が一覧（ルート）', () => {
    expect(backActionOf({ ...base, searching: true })).toBe('clearSearch')
  })

  test('オーバーレイ（ダイアログ・メニュー）、ドロワーは画面より先に閉じる', () => {
    expect(
      backActionOf({ ...base, threadOpen: true, composeMode: 'reply', overlayOpen: true }),
    ).toBe('dismissOverlay')
    expect(backActionOf({ ...base, threadOpen: true, menuOpen: true })).toBe('closeMenu')
    expect(backActionOf({ ...base, menuOpen: true, overlayOpen: true })).toBe('dismissOverlay')
  })
})
