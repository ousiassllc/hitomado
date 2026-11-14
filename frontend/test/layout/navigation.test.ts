import { describe, expect, test } from 'bun:test'
import { backActionOf, type NavState, screenOf } from '../../src/lib/navigation'

const base: NavState = { overlayOpen: false, agentOpen: false, threadOpen: false, searching: false }

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

  test('受信箱以外の見る場所（下書き・送信済み・アーカイブ）は、戻るで受信箱へ戻る。検索結果の戻りが先', () => {
    expect(backActionOf({ ...base, viewOpen: true })).toBe('closeView')
    expect(backActionOf({ ...base, viewOpen: true, searching: true })).toBe('clearSearch')
    expect(backActionOf({ ...base, viewOpen: true, threadOpen: true })).toBe('closeThread')
    expect(backActionOf({ ...base, viewOpen: false })).toBeUndefined()
  })

  test('オーバーレイ（ダイアログ・メニュー）、エージェントのパネルは画面より先に閉じる', () => {
    expect(
      backActionOf({ ...base, threadOpen: true, composeMode: 'reply', overlayOpen: true }),
    ).toBe('dismissOverlay')
    expect(backActionOf({ ...base, threadOpen: true, agentOpen: true })).toBe('closeAgent')
    expect(backActionOf({ ...base, agentOpen: true, overlayOpen: true })).toBe('dismissOverlay')
  })
})

describe('backActionOf（設定の AI のプロバイダのページ。#315）', () => {
  test('プロバイダのページが最前面なら、ダイアログを閉じずに一覧へ戻す', () => {
    expect(backActionOf({ ...base, overlayOpen: true, settingsPageOpen: true })).toBe(
      'closeSettingsPage',
    )
  })

  test('オーバーレイが開いていなければ settingsPageOpen は効かず、ほかのダイアログが前面なら閉じる', () => {
    expect(backActionOf({ ...base, settingsPageOpen: true })).toBeUndefined()
    expect(backActionOf({ ...base, overlayOpen: true, settingsPageOpen: false })).toBe(
      'dismissOverlay',
    )
  })
})
