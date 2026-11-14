import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import {
  callsOf,
  failures,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from '../src/stores/__tests__/settingsKit'

installMocks()
afterAll(restoreMocks)

// mock.module を適用した後で読み込む必要があるため、静的 import にできない。
const { useSettingsStore } = await import('../src/stores/settings')
const { setSendConfirmEnabled } = await import('../src/composables/useSendConfirmSetting')

const baseInfo = {
  version: '0.1.0',
  updateCheckEnabled: true,
  autoUpdateSupported: false,
  autoUpdateEnabled: false,
  sendConfirmEnabled: true,
  dataDir: '/data',
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetKit()
  Object.assign(responses, {
    GetAIConfig: {
      features: {
        draft: { provider: 'anthropic', model: 'm' },
        inboxZero: { provider: '', model: '' },
      },
      apiKeyProviders: null,
      consentedProviders: null,
    },
    GetAppInfo: baseInfo,
    SetSendConfirmEnabled: undefined,
  })
})

describe('settingsStore（送信前の確認画面。#313）', () => {
  test('setSendConfirmEnabled は SetSendConfirmEnabled を呼び、appInfo に反映する（既定は true）', async () => {
    responses.SetSendConfirmEnabled = undefined
    const s = useSettingsStore()
    await s.load()
    expect(s.appInfo?.sendConfirmEnabled).toBe(true)
    await setSendConfirmEnabled(false)
    expect(callsOf('SetSendConfirmEnabled')[0]?.args).toEqual([false])
    expect(s.appInfo?.sendConfirmEnabled).toBe(false)
    await setSendConfirmEnabled(true)
    expect(s.appInfo?.sendConfirmEnabled).toBe(true)
  })

  test('保存に失敗したら appInfo を変えずに投げる', async () => {
    failures.SetSendConfirmEnabled = { cause: { code: 'INTERNAL', message: 'x', retryable: false } }
    const s = useSettingsStore()
    await s.load()
    await expect(setSendConfirmEnabled(false)).rejects.toBeDefined()
    expect(s.appInfo?.sendConfirmEnabled).toBe(true)
  })
})
