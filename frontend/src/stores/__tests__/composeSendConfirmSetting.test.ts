import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  calls,
  count,
  h,
  installMocks,
  names,
  openReply,
  previewOf,
  reset,
  restoreMocks,
  setup,
  useSettingsStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

describe('送信前の確認画面の設定（#313）', () => {
  const info = (sendConfirmEnabled: boolean) =>
    ({
      version: '1',
      updateCheckEnabled: true,
      autoUpdateSupported: false,
      autoUpdateEnabled: false,
      sendConfirmEnabled,
      dataDir: '/d',
    }) as const

  test('オン・設定の読み込み前は、今までどおり確認画面を出し、Send は呼ばない', async () => {
    const { compose, ui } = setup()
    await openReply(compose)
    expect(await compose.prepareSend(false)).toBe(true)
    expect(ui.dialogs.sendConfirm).toBe(true)
    expect(count('Send')).toBe(0)

    compose.cancelPreview()
    useSettingsStore().appInfo = info(true)
    expect(await compose.prepareSend(false)).toBe(true)
    expect(ui.dialogs.sendConfirm).toBe(true)
    expect(count('Send')).toBe(0)
  })

  test('オフで警告が無いときは、確認画面を出さず、PrepareSend の確認トークンでそのまま送る', async () => {
    const { compose, ui } = setup()
    useSettingsStore().appInfo = info(false)
    await openReply(compose)
    expect(await compose.prepareSend(false)).toBe(true)
    expect(ui.dialogs.sendConfirm).toBe(false)
    expect(compose.preview).toBeUndefined()
    expect(names().filter((n) => n === 'PrepareSend' || n === 'Send')).toEqual([
      'PrepareSend',
      'Send',
    ])
    const send = calls.find((c) => c.name === 'Send')
    expect(send?.args[0]).toBe('s1')
    expect(send?.args[1]).toBe('t1')
    expect(compose.session).toBeUndefined()
    expect(ui.toast).toMatchObject({ kind: 'success', message: '送信しました' })
  })

  test('オフでも、準備に失敗したら送らない（送信元未選択・宛先なしなどの検証は省かない）', async () => {
    const { compose } = setup()
    useSettingsStore().appInfo = info(false)
    await openReply(compose)
    h.failures.PrepareSend = appError('NO_RECIPIENTS')
    expect(await compose.prepareSend(false)).toBe(false)
    expect(count('Send')).toBe(0)
    expect(compose.session).toBeDefined()
  })

  test.each([['OTHER_OWN_ACCOUNT'], ['EMPTY_SUBJECT'], ['EMPTY_BODY']])(
    'オフでも %s の警告があるときは確認画面を出し、Send は呼ばない',
    async (code) => {
      const { compose, ui } = setup()
      useSettingsStore().appInfo = info(false)
      await openReply(compose)
      h.responses.PrepareSend = { ...previewOf(), warnings: [{ code, message: 'w' }] }
      expect(await compose.prepareSend(false)).toBe(true)
      expect(ui.dialogs.sendConfirm).toBe(true)
      expect(compose.preview?.warnings).toHaveLength(1)
      expect(count('Send')).toBe(0)
    },
  )

  test('オフの送信が失敗したら、本文・宛先を保ったままエラーを出し、確認トークンは捨てる', async () => {
    const { compose, ui } = setup()
    useSettingsStore().appInfo = info(false)
    await openReply(compose)
    h.failures.Send = appError('NETWORK')
    expect(await compose.prepareSend(false)).toBe(false)
    expect(ui.dialogs.sendConfirm).toBe(false)
    expect(compose.session).toBeDefined()
    expect(compose.preview).toBeUndefined()
    expect(compose.error).toBeTruthy()
    expect(compose.content.to).toHaveLength(1)
  })

  test('オフの送信中は二重に送らない', async () => {
    const { compose } = setup()
    useSettingsStore().appInfo = info(false)
    await openReply(compose)
    const gate = Promise.withResolvers<void>()
    h.gates.Send = gate.promise
    const first = compose.prepareSend(false)
    // Send がゲートで止まるまで（上限つきで）マイクロタスクを回す。固定時間は待たない。
    for (let i = 0; i < 100 && count('Send') === 0; i++) await Promise.resolve()
    expect(count('Send')).toBe(1)
    expect(await compose.prepareSend(false)).toBe(false)
    gate.resolve()
    await first
    expect(count('Send')).toBe(1)
  })
})
