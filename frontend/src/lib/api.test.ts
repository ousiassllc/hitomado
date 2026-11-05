import { describe, expect, test } from 'bun:test'
import { type AppError, parseBindingError } from './api'

const APP_ERROR: AppError = {
  code: 'OFFLINE',
  message: 'ネットワークに接続できません。',
  retryable: true,
}

describe('parseBindingError', () => {
  test('cause に apperr の JSON がある（Windows の転送路）', () => {
    const e = Object.assign(new Error('INTERNAL: x'), {
      cause: JSON.stringify({ ...APP_ERROR, accountId: 'a1' }),
    })
    expect(parseBindingError(e)).toEqual({ ...APP_ERROR, accountId: 'a1' })
  })

  // Android の Wails の転送路は cause を捨て、文面だけを返す。全てが INTERNAL の定型文になっていた（#185）。
  test('Android の転送路: メッセージの文字列からコードと文面を取り出す', () => {
    const e = new Error(
      'Binding call failed: Bound method returned an error: AI_FAILED: ChatGPT のログイン処理に失敗しました。',
    )
    expect(parseBindingError(e)).toEqual({
      code: 'AI_FAILED',
      message: 'ChatGPT のログイン処理に失敗しました。',
      retryable: false,
    })
  })

  test('Android の転送路: 再試行できるコードは retryable になる', () => {
    for (const code of ['OFFLINE', 'RATE_LIMITED', 'OAUTH_TIMEOUT']) {
      const e = new Error(`Binding call failed: Bound method returned an error: ${code}: 文面。`)
      expect(parseBindingError(e).retryable).toBe(true)
    }
  })

  test('Android の転送路: 未知のコードは INTERNAL の定型文に落とす', () => {
    const e = new Error(
      'Binding call failed: Bound method returned an error: SOMETHING_NEW: 文面。',
    )
    expect(parseBindingError(e)).toEqual({
      code: 'INTERNAL',
      message: '予期しないエラーが発生しました。',
      retryable: false,
    })
  })

  test('解釈できない値は INTERNAL の定型文', () => {
    expect(parseBindingError(new Error('boom'))).toEqual({
      code: 'INTERNAL',
      message: '予期しないエラーが発生しました。',
      retryable: false,
    })
    expect(parseBindingError(undefined).code).toBe('INTERNAL')
  })
})
