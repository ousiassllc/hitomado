import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realBindings from '../bindings/github.com/ousiassllc/hitomado/internal/app'
import * as realEvents from '../src/lib/events'

const BINDINGS = '../bindings/github.com/ousiassllc/hitomado/internal/app'

type Handler = (payload: unknown) => void
const handlers = new Map<string, Handler>()
const unsubscribed: string[] = []
const calls: { name: string; args: unknown[] }[] = []
let responses: Record<string, unknown> = {}
let failures: Record<string, unknown> = {}

function binding(name: string) {
  return (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in failures) return Promise.reject(failures[name])
    return Promise.resolve(responses[name])
  }
}

mock.module(BINDINGS, () => ({
  AIService: {
    GetAIConfig: binding('GetAIConfig'),
    SetAIConfig: binding('SetAIConfig'),
    SetAIApiKey: binding('SetAIApiKey'),
    ClearAIApiKey: binding('ClearAIApiKey'),
    RecordAIConsent: binding('RecordAIConsent'),
  },
  AppService: {
    GetAppInfo: binding('GetAppInfo'),
    SetUpdateCheckEnabled: binding('SetUpdateCheckEnabled'),
    CheckForUpdate: binding('CheckForUpdate'),
  },
  ThreadService: { OpenExternal: binding('OpenExternal') },
  MCPService: {
    GetMCPStatus: binding('GetMCPStatus'),
    SetMCPConfig: binding('SetMCPConfig'),
    IssueMCPToken: binding('IssueMCPToken'),
    RevokeMCPToken: binding('RevokeMCPToken'),
  },
  LogService: { OpenLogFolder: binding('OpenLogFolder'), ExportLogs: binding('ExportLogs') },
}))

mock.module('../src/lib/events', () => ({
  ...realEvents,
  onEvent: (name: string, h: Handler) => {
    handlers.set(name, h)
    return () => {
      unsubscribed.push(name)
      handlers.delete(name)
    }
  },
}))

// mock.module を適用した後で読み込む必要があるため、静的 import にできない。
// 他のテストファイルへモックが漏れないよう、終了時に本物へ戻す。
afterAll(() => {
  mock.module(BINDINGS, () => ({ ...realBindings }))
  mock.module('../src/lib/events', () => ({ ...realEvents }))
})

const { useSettingsStore } = await import('../src/stores/settings')
const { useUiStore } = await import('../src/stores/ui')

const baseAI = { provider: 'anthropic', model: 'm', hasApiKey: false, consentedProviders: null }
const baseMcp = {
  enabled: false,
  host: '127.0.0.1',
  port: 47650,
  allowNonLoopback: false,
  hasToken: false,
  listening: false,
  endpoint: 'http://127.0.0.1:47650/mcp',
}
const baseInfo = { version: '0.1.0', updateCheckEnabled: true, dataDir: '/data' }

function callsOf(name: string) {
  return calls.filter((c) => c.name === name)
}

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
  unsubscribed.length = 0
  calls.length = 0
  failures = {}
  responses = {
    GetAIConfig: baseAI,
    GetAppInfo: baseInfo,
    SetAIConfig: { ...baseAI, provider: 'openai', model: 'x', baseUrl: 'https://llm.example/v1' },
    SetAIApiKey: { ...baseAI, hasApiKey: true, consentedProviders: ['anthropic'] },
    ClearAIApiKey: baseAI,
    RecordAIConsent: { ...baseAI, consentedProviders: ['anthropic'] },
    SetUpdateCheckEnabled: undefined,
    CheckForUpdate: { available: true, version: '0.2.0', url: 'https://example.com/r' },
    OpenExternal: undefined,
    GetMCPStatus: baseMcp,
    SetMCPConfig: { ...baseMcp, enabled: true, listening: true },
    IssueMCPToken: { token: 'hmcp_example', status: { ...baseMcp, hasToken: true } },
    RevokeMCPToken: baseMcp,
    OpenLogFolder: undefined,
    ExportLogs: { saved: true, path: '/tmp/logs.zip' },
  }
})

describe('settingsStore', () => {
  test('load は AI 設定とアプリ情報を取得し、consentedProviders の null を空配列にする', async () => {
    const s = useSettingsStore()
    await s.load()
    expect(s.aiConfig?.consentedProviders).toEqual([])
    expect(s.appInfo?.dataDir).toBe('/data')
  })

  test('load 失敗時はエラートーストを出し、例外にしない', async () => {
    failures.GetAIConfig = { code: 'INTERNAL', message: '読み込めません', retryable: false }
    const s = useSettingsStore()
    await s.load()
    expect(s.aiConfig).toBeUndefined()
    const ui = useUiStore()
    expect(ui.toasts.at(-1)).toMatchObject({ kind: 'error', message: '読み込めません' })
  })

  test('setAIConfig・setAIApiKey・clearAIApiKey・recordAIConsent は戻り値で aiConfig を更新する', async () => {
    const s = useSettingsStore()
    await s.setAIConfig('openai', 'x', 'https://llm.example/v1')
    expect(callsOf('SetAIConfig')[0]?.args).toEqual(['openai', 'x', 'https://llm.example/v1'])
    expect(s.aiConfig?.baseUrl).toBe('https://llm.example/v1')

    await s.setAIApiKey('anthropic', 'secret')
    expect(callsOf('SetAIApiKey')[0]?.args).toEqual(['anthropic', 'secret'])
    expect(s.aiConfig?.hasApiKey).toBe(true)

    await s.clearAIApiKey('anthropic')
    expect(s.aiConfig?.hasApiKey).toBe(false)

    await s.recordAIConsent('anthropic')
    expect(s.aiConfig?.consentedProviders).toEqual(['anthropic'])
  })

  test('AI 設定の保存失敗は AppError として投げ直し、state を変えない', async () => {
    const s = useSettingsStore()
    await s.load()
    failures.SetAIConfig = { cause: { code: 'VALIDATION', message: '不正', retryable: false } }
    await expect(s.setAIConfig('openai', '', '')).rejects.toMatchObject({ code: 'VALIDATION' })
    expect(s.aiConfig?.provider).toBe('anthropic')
  })

  test('setUpdateCheckEnabled(false) は appInfo を更新し、更新バナーと update を消す', async () => {
    const s = useSettingsStore()
    const ui = useUiStore()
    await s.load()
    await s.checkUpdate()
    expect(ui.banners.some((b) => b.key === 'update')).toBe(true)

    await s.setUpdateCheckEnabled(false)
    expect(callsOf('SetUpdateCheckEnabled')[0]?.args).toEqual([false])
    expect(s.appInfo?.updateCheckEnabled).toBe(false)
    expect(s.update).toBeUndefined()
    expect(ui.banners.some((b) => b.key === 'update')).toBe(false)
  })

  test('checkUpdate は更新ありでバナーを設定し、更新なしでバナーを消す', async () => {
    const s = useSettingsStore()
    const ui = useUiStore()
    const info = await s.checkUpdate()
    expect(info.available).toBe(true)
    expect(s.update?.version).toBe('0.2.0')
    const banner = ui.banners.find((b) => b.key === 'update')
    expect(banner?.kind).toBe('update')
    expect(banner?.message).toContain('0.2.0')

    responses.CheckForUpdate = { available: false }
    await s.checkUpdate()
    expect(s.update?.available).toBe(false)
    expect(ui.banners.some((b) => b.key === 'update')).toBe(false)
  })

  test('update:available を購読して update とバナーを更新し、バナーの操作は OpenExternal で開く', async () => {
    const s = useSettingsStore()
    const ui = useUiStore()
    await s.start()
    const h = handlers.get('update:available')
    expect(h).toBeDefined()
    h?.({ version: '0.3.0', url: 'https://example.com/v0.3.0' })
    expect(s.update).toEqual({
      available: true,
      version: '0.3.0',
      url: 'https://example.com/v0.3.0',
    })

    const banner = ui.banners.find((b) => b.key === 'update')
    expect(banner?.message).toContain('0.3.0')
    banner?.action?.run()
    await Promise.resolve()
    expect(callsOf('OpenExternal')[0]?.args).toEqual(['https://example.com/v0.3.0'])
  })

  test('start は更新確認が ON ならイベント前でも一度確認し、更新ありならバナーを出す', async () => {
    const s = useSettingsStore()
    const ui = useUiStore()
    await s.start()
    expect(callsOf('CheckForUpdate')).toHaveLength(1)
    expect(s.update?.version).toBe('0.2.0')
    expect(ui.banners.find((b) => b.key === 'update')?.message).toContain('0.2.0')
  })

  test('start は更新確認が OFF なら確認しない', async () => {
    responses.GetAppInfo = { ...baseInfo, updateCheckEnabled: false }
    const s = useSettingsStore()
    await s.start()
    expect(callsOf('CheckForUpdate')).toHaveLength(0)
    expect(s.update).toBeUndefined()
  })

  test('start 時の確認が更新なし・失敗でも、トーストを出さず既存の update を消さない', async () => {
    const s = useSettingsStore()
    const ui = useUiStore()
    responses.CheckForUpdate = { available: false }
    await s.start()
    expect(s.update).toBeUndefined()
    s.stop()
    failures.CheckForUpdate = { code: 'OFFLINE', message: 'オフライン', retryable: true }
    await s.start()
    expect(ui.toasts).toHaveLength(0)
    expect(ui.banners).toHaveLength(0)
  })

  test('setUpdateCheckEnabled(true) は即座に更新確認し、更新ありならバナーを出す', async () => {
    responses.GetAppInfo = { ...baseInfo, updateCheckEnabled: false }
    const s = useSettingsStore()
    const ui = useUiStore()
    await s.load()
    await s.setUpdateCheckEnabled(true)
    expect(callsOf('CheckForUpdate')).toHaveLength(1)
    expect(s.appInfo?.updateCheckEnabled).toBe(true)
    expect(ui.banners.some((b) => b.key === 'update')).toBe(true)
  })

  test('setUpdateCheckEnabled(false) は更新確認を呼ばない', async () => {
    const s = useSettingsStore()
    await s.setUpdateCheckEnabled(false)
    expect(callsOf('CheckForUpdate')).toHaveLength(0)
  })

  test('start は冪等で、stop で購読を解除する', async () => {
    const s = useSettingsStore()
    await s.start()
    await s.start()
    expect(callsOf('GetAIConfig')).toHaveLength(1)
    expect(callsOf('CheckForUpdate')).toHaveLength(1)
    s.stop()
    expect(unsubscribed).toEqual(['update:available'])
  })

  test('openExternal の失敗はエラートーストにする', async () => {
    failures.OpenExternal = { code: 'VALIDATION', message: '開けません', retryable: false }
    const s = useSettingsStore()
    await s.openExternal('ftp://x')
    expect(useUiStore().toasts.at(-1)).toMatchObject({ kind: 'error', message: '開けません' })
  })

  test('MCP: 既定はオフ。設定の反映・トークンの発行と無効化は戻り値の状態で更新し、トークンの値は状態に残さない', async () => {
    const s = useSettingsStore()
    await s.loadMcp()
    expect(s.mcp).toMatchObject({ enabled: false, listening: false, hasToken: false })

    await s.setMcpConfig({ enabled: true, host: '127.0.0.1', port: 47650, allowNonLoopback: false })
    expect(callsOf('SetMCPConfig')[0]?.args).toEqual([
      { enabled: true, host: '127.0.0.1', port: 47650, allowNonLoopback: false },
    ])
    expect(s.mcp?.listening).toBe(true)

    const token = await s.issueMcpToken()
    expect(token).toBe('hmcp_example')
    expect(s.mcp?.hasToken).toBe(true)
    expect(JSON.stringify(s.mcp)).not.toContain('hmcp_example')

    await s.revokeMcpToken()
    expect(s.mcp?.hasToken).toBe(false)
  })

  test('MCP: 設定の検証エラーは AppError として投げ直し、状態を変えない', async () => {
    const s = useSettingsStore()
    await s.loadMcp()
    failures.SetMCPConfig = { code: 'VALIDATION', message: '不正', retryable: false }
    await expect(
      s.setMcpConfig({ enabled: true, host: '0.0.0.0', port: 47650, allowNonLoopback: false }),
    ).rejects.toMatchObject({ code: 'VALIDATION' })
    expect(s.mcp?.enabled).toBe(false)
  })

  test('ログ: フォルダを開く・書き出す', async () => {
    const s = useSettingsStore()
    await s.openLogFolder()
    expect(callsOf('OpenLogFolder')).toHaveLength(1)
    expect(await s.exportLogs()).toEqual({ saved: true, path: '/tmp/logs.zip' })
  })
})
