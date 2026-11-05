import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { installMocks, openReply, reset, restoreMocks, setup } from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

// 「AI への指示」（#109）は作成セッションのメモリにだけ持つ。下書きの保存内容には含めず、セッションを閉じる・破棄すると消える。
describe('aiInstruction', () => {
  test('初期は空で、入力しても編集（dirty・自動保存）にはならない', async () => {
    const { compose } = setup()
    await openReply(compose)
    expect(compose.aiInstruction).toBe('')
    compose.aiInstruction = '断る'
    expect(compose.dirty).toBe(false)
  })

  test('閉じると消える（次のセッションに引き継がない）', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.aiInstruction = '3 行で短く'
    expect(await compose.close()).toBe(true)
    expect(compose.aiInstruction).toBe('')
  })

  test('破棄すると消える', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.aiInstruction = '3 行で短く'
    expect(await compose.discard()).toBe(true)
    expect(compose.aiInstruction).toBe('')
  })
})
