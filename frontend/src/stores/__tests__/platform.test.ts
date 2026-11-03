import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realPlatform from '../../../bindings/github.com/ousiassllc/hitomado/internal/capability/platformservice'
import { useUiStore } from '../ui'

const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/capability'

const ANDROID = {
  os: 'android',
  supportsMcp: false,
  supportsTray: false,
  supportsAutoUpdate: false,
  supportsWindowState: false,
  singleInstance: false,
}

let calls = 0
let result: () => Promise<unknown> = async () => ANDROID

mock.module(`${B}/platformservice`, () => ({
  GetCapabilities: async () => {
    calls++
    return result()
  },
}))
afterAll(() => {
  mock.module(`${B}/platformservice`, () => ({ ...realPlatform }))
})

const { usePlatformStore } = await import('../platform')

beforeEach(() => {
  setActivePinia(createPinia())
  calls = 0
  result = async () => ANDROID
})

describe('usePlatformStore', () => {
  test('読み込み前はデスクトップの機能（全機能あり）', () => {
    const p = usePlatformStore()
    expect(p.loaded).toBe(false)
    expect(p.supportsMcp).toBe(true)
    expect(p.supportsTray).toBe(true)
    expect(p.supportsAutoUpdate).toBe(true)
    expect(p.supportsWindowState).toBe(true)
    expect(p.singleInstance).toBe(true)
  })

  test('start は Go の機能を読み、Android では各機能が無効になる。2 回呼んでも読むのは 1 回', async () => {
    const p = usePlatformStore()
    await p.start()
    await p.start()
    expect(calls).toBe(1)
    expect(p.loaded).toBe(true)
    expect(p.capabilities.os).toBe('android')
    expect(p.supportsMcp).toBe(false)
    expect(p.supportsTray).toBe(false)
    expect(p.supportsAutoUpdate).toBe(false)
    expect(p.supportsWindowState).toBe(false)
    expect(p.singleInstance).toBe(false)
  })

  test('読み込みに失敗したらエラートーストを出し、既定（デスクトップ）のまま', async () => {
    result = async () => {
      throw new Error('boom')
    }
    const p = usePlatformStore()
    await p.start()
    expect(p.loaded).toBe(false)
    expect(p.supportsMcp).toBe(true)
    expect(useUiStore().toasts.at(-1)?.kind).toBe('error')
  })
})
