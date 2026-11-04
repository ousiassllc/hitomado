import { describe, expect, test } from 'bun:test'
import { buildUpdateBanner, type UpdateBannerInput } from './updateBanner'

const opened: string[] = []
let restarted = 0
function input(over: Partial<UpdateBannerInput>): UpdateBannerInput {
  return {
    info: { available: true, version: '0.2.0', url: 'https://example.com/r' },
    status: { stage: 'none' },
    autoUpdate: true,
    openExternal: (u) => opened.push(u),
    restart: () => {
      restarted++
    },
    ...over,
  }
}

describe('buildUpdateBanner', () => {
  test('自動アップデートを使わないときは、新しい版のリンクだけを出す', () => {
    const b = buildUpdateBanner(
      input({ autoUpdate: false, status: { stage: 'ready', version: '0.2.0' } }),
    )
    expect(b?.action?.label).toBe('ダウンロードページを開く')
    b?.action?.run()
    expect(opened).toEqual(['https://example.com/r'])
  })

  test('新しい版がなく、進みもなければ出さない', () => {
    expect(buildUpdateBanner(input({ info: { available: false } }))).toBeUndefined()
    expect(buildUpdateBanner(input({ info: undefined }))).toBeUndefined()
  })

  test('ready は「再起動して更新」。注意の文面（前回の入れ替えが完了しなかった等）を添える', () => {
    const b = buildUpdateBanner(
      input({
        status: { stage: 'ready', version: '0.2.0', message: '前回は完了しませんでした。' },
      }),
    )
    expect(b?.message).toContain('0.2.0')
    expect(b?.message).toContain('前回は完了しませんでした。')
    restarted = 0
    b?.action?.run()
    expect(restarted).toBe(1)
  })

  test('ダウンロード中はボタンなし', () => {
    const b = buildUpdateBanner(input({ status: { stage: 'downloading', version: '0.2.0' } }))
    expect(b?.action).toBeUndefined()
    expect(b?.message).toContain('ダウンロードしています')
  })

  test('失敗は文面とリンク。リンクが無ければボタンなし', () => {
    const failed = { stage: 'failed', version: '0.2.0', message: '失敗しました。' } as const
    expect(buildUpdateBanner(input({ status: failed }))?.action?.label).toBe(
      'ダウンロードページを開く',
    )
    const b = buildUpdateBanner(
      input({ status: failed, info: { available: true, version: '0.2.0' } }),
    )
    expect(b?.message).toBe('失敗しました。')
    expect(b?.action).toBeUndefined()
  })
})
