import { describe, expect, test } from 'bun:test'
import { type AiDraftDeps, useAiDraft } from './useAiDraft'

type Config = ReturnType<AiDraftDeps['config']>

function setup(over: { config?: Config; hasBody?: boolean } = {}) {
  const log: string[] = []
  const generated: string[] = []
  const gens: PromiseWithResolvers<{ bodyHtml: string }>[] = []
  let config: Config =
    'config' in over
      ? over.config
      : {
          provider: 'anthropic',
          configured: true,
          hasApiKey: true,
          consentedProviders: ['anthropic'],
        }
  const ai = useAiDraft({
    config: () => config,
    hasBody: () => over.hasBody ?? false,
    recordConsent: async (p) => {
      log.push(`consent:${p}`)
      config = config && {
        ...config,
        consentedProviders: [...(config.consentedProviders ?? []), p],
      }
    },
    generate: (id) => {
      log.push(`generate:${id}`)
      const d = Promise.withResolvers<{ bodyHtml: string }>()
      gens.push(d)
      return d.promise
    },
    cancel: async (id) => {
      log.push(`cancel:${id}`)
    },
    openSetup: (c) => log.push(`setup:${c}`),
    onGenerated: (h) => generated.push(h),
    newRequestId: () => `r${gens.length + 1}`,
  })
  return { ai, log, generated, gens }
}

// マクロタスク 1 回分、保留中の Promise 連鎖を進める（固定時間の待ちではない）
const tick = () => new Promise<void>((r) => setTimeout(r, 0))

describe('useAiDraft 初回の案内（#104）', () => {
  const fresh: Config = {
    provider: 'chatgpt',
    configured: false,
    hasApiKey: false,
    consentedProviders: [],
  }

  test('AI の設定が未保存のとき、ボタンを押すと初回の案内になる。生成も同意の記録もしない', async () => {
    const { ai, log } = setup({ config: fresh })
    expect(ai.needsSetup.value).toBe(true)
    expect(ai.needsKey.value).toBe(false) // ボタンは無効にしない（押すと案内が出る）
    await ai.start()
    expect(ai.step.value).toBe('setup')
    expect(log).toEqual([])
  })

  test('「Continue with ChatGPT」は設定画面の ChatGPT の流れへ、「別のプロバイダ」はプロバイダの選択へ進む。どちらも下書きは作らない', async () => {
    for (const choice of ['chatgpt', 'other'] as const) {
      const { ai, log } = setup({ config: fresh })
      await ai.start()
      ai.answerSetup(choice)
      expect(ai.step.value).toBe('idle')
      expect(log).toEqual([`setup:${choice}`])
    }
  })

  test('「あとで」は何もせず閉じる。もう一度押せば、また案内が出る', async () => {
    const { ai, log } = setup({ config: fresh })
    await ai.start()
    ai.answerSetup()
    expect(ai.step.value).toBe('idle')
    expect(log).toEqual([])
    await ai.start()
    expect(ai.step.value).toBe('setup')
  })

  test('保存済みの設定（anthropic・openai・chatgpt）では案内を出さず、従来どおりの流れ', async () => {
    for (const provider of ['anthropic', 'openai', 'chatgpt']) {
      const { ai, log } = setup({
        config: { provider, configured: true, hasApiKey: true, consentedProviders: [provider] },
      })
      expect(ai.needsSetup.value).toBe(false)
      void ai.start() // 生成は結果が返るまで終わらない
      await tick()
      expect(ai.step.value).toBe('generating')
      expect(log).toEqual(['generate:r1'])
    }
    // キー未設定（保存済み）は、従来どおりボタンを無効にして設定へ誘導する。
    const { ai } = setup({
      config: { provider: 'openai', configured: true, hasApiKey: false, consentedProviders: [] },
    })
    expect(ai.needsKey.value).toBe(true)
    expect(ai.needsSetup.value).toBe(false)
  })
})

describe('useAiDraft', () => {
  test('キー未設定では何も始めない（needsKey）', async () => {
    const { ai, log } = setup({
      config: { provider: 'anthropic', configured: true, hasApiKey: false, consentedProviders: [] },
    })
    expect(ai.needsKey.value).toBe(true)
    await ai.start()
    expect(ai.step.value).toBe('idle')
    expect(log).toEqual([])
  })

  test('設定が未ロードでもキー未設定として扱う', () => {
    expect(setup({ config: undefined }).ai.needsKey.value).toBe(true)
  })

  test('未同意なら同意ダイアログ → 同意を記録して生成する', async () => {
    const { ai, log, gens, generated } = setup({
      config: {
        provider: 'openai',
        configured: true,
        hasApiKey: true,
        consentedProviders: ['anthropic'],
      },
    })
    await ai.start()
    expect(ai.step.value).toBe('consent')
    expect(log).toEqual([])
    const p = ai.answerConsent(true)
    await tick()
    await tick()
    expect(log).toEqual(['consent:openai', 'generate:r1'])
    expect(ai.step.value).toBe('generating')
    gens[0].resolve({ bodyHtml: '<p>下書き</p>' })
    await p
    expect(generated).toEqual(['<p>下書き</p>'])
    expect(ai.step.value).toBe('idle')
  })

  test('同意しなければ生成しない', async () => {
    const { ai, log } = setup({
      config: { provider: 'openai', configured: true, hasApiKey: true, consentedProviders: [] },
    })
    await ai.start()
    await ai.answerConsent(false)
    expect(ai.step.value).toBe('idle')
    expect(log).toEqual([])
  })

  test('本文入力済みなら置換確認。キャンセルなら生成しない', async () => {
    const { ai, log } = setup({ hasBody: true })
    await ai.start()
    expect(ai.step.value).toBe('replace')
    await ai.answerReplace(false)
    expect(ai.step.value).toBe('idle')
    expect(log).toEqual([])
  })

  test('置換を承諾すると生成し、結果を onGenerated に渡す', async () => {
    const { ai, gens, generated } = setup({ hasBody: true })
    await ai.start()
    const p = ai.answerReplace(true)
    expect(ai.step.value).toBe('generating')
    gens[0].resolve({ bodyHtml: '<p>x</p>' })
    await p
    expect(generated).toEqual(['<p>x</p>'])
  })

  test('失敗はエラー表示のみで本文（onGenerated）に触れない', async () => {
    const { ai, gens, generated } = setup()
    const p = ai.start()
    gens[0].reject({ code: 'AI_FAILED', message: 'キーが不正です', retryable: false })
    await p
    expect(ai.error.value).toBe('キーが不正です')
    expect(generated).toEqual([])
    expect(ai.step.value).toBe('idle')
  })

  test('生成中はキャンセルできる。結果もエラーも表示しない', async () => {
    const { ai, log, gens, generated } = setup()
    const p = ai.start()
    expect(ai.step.value).toBe('generating')
    await ai.cancel()
    expect(log).toEqual(['generate:r1', 'cancel:r1'])
    gens[0].reject({ code: 'CANCELLED', message: 'キャンセルされました', retryable: false })
    await p
    expect(ai.error.value).toBeUndefined()
    expect(generated).toEqual([])
    expect(ai.step.value).toBe('idle')
  })

  test('キャンセル後に結果が届いても採用しない', async () => {
    const { ai, gens, generated } = setup()
    const p = ai.start()
    await ai.cancel()
    gens[0].resolve({ bodyHtml: '<p>late</p>' })
    await p
    expect(generated).toEqual([])
  })

  test('生成中は二重に開始しない', async () => {
    const { ai, log, gens } = setup()
    const p = ai.start()
    await ai.start()
    expect(log).toEqual(['generate:r1'])
    gens[0].resolve({ bodyHtml: '' })
    await p
  })

  test('次の開始でエラー表示を消す', async () => {
    const { ai, gens } = setup()
    const p = ai.start()
    gens[0].reject({ code: 'AI_FAILED', message: 'x', retryable: false })
    await p
    expect(ai.error.value).toBe('x')
    const p2 = ai.start()
    expect(ai.error.value).toBeUndefined()
    gens[1].resolve({ bodyHtml: '' })
    await p2
  })
})
