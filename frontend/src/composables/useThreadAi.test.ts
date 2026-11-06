import { describe, expect, test } from 'bun:test'
import type { AiPrecheckConfig } from '../lib/aiPrecheck'
import { type ThreadAiAction, useThreadAi } from './useThreadAi'

const READY: AiPrecheckConfig = {
  provider: 'anthropic',
  configured: true,
  hasApiKey: true,
  consentedProviders: ['anthropic'],
}

function setup(initial: AiPrecheckConfig = READY) {
  let config: AiPrecheckConfig | undefined = initial
  const runs: ThreadAiAction[] = []
  const log: string[] = []
  const ai = useThreadAi({
    config: () => config,
    openSetup: (choice) => log.push(`setup:${choice}`),
    recordConsent: async (provider) => {
      log.push(`consent:${provider}`)
      config = { ...READY, consentedProviders: [provider] }
    },
    run: async (action) => {
      runs.push(action)
    },
  })
  return { ai, runs, log, setConfig: (c: AiPrecheckConfig | undefined) => (config = c) }
}

describe('useThreadAi', () => {
  test('前提が揃っていれば押された操作をそのまま実行する（要約・質問とも）', async () => {
    const { ai, runs } = setup()
    await ai.start('summarize')
    await ai.start('ask')
    expect(runs).toEqual(['summarize', 'ask'])
    expect(ai.step.value).toBe('idle')
  })

  test('AI の設定が未保存なら、実行せず初回の案内へ。「あとで」では何もしない', async () => {
    const { ai, runs, log } = setup({ ...READY, configured: false, hasApiKey: false })
    await ai.start('summarize')
    expect(ai.step.value).toBe('setup')
    expect(runs).toEqual([])
    ai.answerSetup()
    expect(ai.step.value).toBe('idle')
    expect(log).toEqual([])

    await ai.start('ask')
    ai.answerSetup('chatgpt')
    expect(log).toEqual(['setup:chatgpt'])
    expect(runs).toEqual([]) // 設定の流れから戻ったら、もう一度押す
  })

  test('キー未設定（ChatGPT は未ログイン）では実行しない。判定は needsKey で公開する', async () => {
    const { ai, runs } = setup({ ...READY, hasApiKey: false })
    expect(ai.needsKey.value).toBe(true)
    await ai.start('summarize')
    expect(runs).toEqual([])
    expect(ai.step.value).toBe('idle')
  })

  test('未同意なら同意の画面へ。同意すると記録して、押された操作を続ける', async () => {
    const { ai, runs, log } = setup({ ...READY, consentedProviders: [] })
    expect(ai.needsConsent.value).toBe(true)
    await ai.start('ask')
    expect(ai.step.value).toBe('consent')
    expect(runs).toEqual([])
    await ai.answerConsent(true)
    expect(log).toEqual(['consent:anthropic'])
    expect(runs).toEqual(['ask']) // 要約ではなく、押された質問を続ける
    expect(ai.step.value).toBe('idle')
  })

  test('同意しない・記録に失敗したときは実行せず、失敗は error に出す', async () => {
    const a = setup({ ...READY, consentedProviders: [] })
    await a.ai.start('summarize')
    await a.ai.answerConsent(false)
    expect(a.runs).toEqual([])
    expect(a.ai.step.value).toBe('idle')

    const b = useThreadAi({
      config: () => ({ ...READY, consentedProviders: [] }),
      openSetup: () => {},
      recordConsent: async () => {
        throw {
          cause: JSON.stringify({ code: 'INTERNAL', message: '保存できません', retryable: false }),
        }
      },
      run: async () => {
        throw new Error('呼ばれない')
      },
    })
    await b.start('summarize')
    await b.answerConsent(true)
    expect(b.error.value).toBe('保存できません')
    expect(b.step.value).toBe('idle')
  })

  test('確認の途中（案内・同意の画面）では新しい操作を受け付けない', async () => {
    const { ai, runs, setConfig } = setup({ ...READY, consentedProviders: [] })
    await ai.start('summarize')
    setConfig(READY)
    await ai.start('ask')
    expect(runs).toEqual([])
    expect(ai.step.value).toBe('consent')
  })
})
