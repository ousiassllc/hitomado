import { beforeEach, describe, expect, test } from 'bun:test'
import { useUiStore } from '../../../stores/ui'
import {
  acctA,
  base,
  button,
  chatgptCfg,
  consentEl,
  dialogButton,
  flushPromises,
  mountAI,
  setup,
  until,
} from './aiTestSupport'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

describe('AISettings（つなぎ先のページ。#376）', () => {
  test('モデル・エフォート・「設定を保存」は出さない（モデルは「機能ごとに使う AI」で選ぶ）', () => {
    const { pinia } = setup()
    const w = mountAI(pinia)
    expect(w.find('[data-testid="ai-model"]').exists()).toBe(false)
    expect(w.find('[data-testid="ai-effort"]').exists()).toBe(false)
    expect(w.text()).not.toContain('設定を保存')
    expect(w.text()).not.toContain('一覧を更新')
    expect(w.find('input[type="password"]').exists()).toBe(true)
  })

  test('保存済みの送信先（プロバイダ・Base URL）を常に表示する。入力中の Base URL が違えば未保存と示す', async () => {
    const { pinia } = setup({ baseUrl: 'https://llm.example/v1' })
    const w = mountAI(pinia)
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://llm.example/v1')
    await w.get('input[type="url"]').setValue('https://other.example/v1')
    expect(w.text()).toContain('入力中の送信先')
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://llm.example/v1')
  })

  test('Base URL は「Base URL を保存」（Enter でも）で保存する。変えるまでは押せない。モデル一覧は取得しない', async () => {
    const { pinia, store, calls } = setup()
    const saved: string[] = []
    store.setAIBaseURL = async (u: string) => {
      saved.push(u)
    }
    const w = mountAI(pinia)
    expect(button(w, 'Base URL を保存').attributes('disabled')).toBeDefined()
    const url = w.get('input[type="url"]')
    await url.setValue(' https://llm.example/v1 ')
    expect(button(w, 'Base URL を保存').attributes('disabled')).toBeUndefined()
    await url.trigger('keydown.enter')
    await flushPromises()
    expect(saved).toEqual(['https://llm.example/v1'])
    expect(calls).toEqual([])
  })

  test('Base URL の保存に失敗したら理由を表示する', async () => {
    const { pinia, store } = setup()
    store.setAIBaseURL = async () => {
      throw {
        code: 'VALIDATION',
        message: 'Base URL は http:// または https:// で始まる URL を指定してください。',
        retryable: false,
      }
    }
    const w = mountAI(pinia)
    await w.get('input[type="url"]').setValue('ftp://x')
    await button(w, 'Base URL を保存').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('Base URL は http')
  })

  test('API キーを保存するとキーは入力欄から消え、未同意なら同意の画面が出る。同意は保存済みの Base URL に記録する', async () => {
    const { pinia, store } = setup({
      apiKeyProviders: [],
      consentedProviders: [],
      baseUrl: 'https://llm.example/v1',
    })
    const keys: string[] = []
    const agreed: string[][] = []
    store.setAIApiKey = async (_p: string, k: string) => {
      keys.push(k)
    }
    store.recordAIConsent = async (p: string, url = '') => {
      agreed.push([p, url])
    }
    const w = mountAI(pinia)
    await w.get('input[type="password"]').setValue('sk-secret-value')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(keys).toEqual(['sk-secret-value'])
    expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('sk-secret-value')
    expect(consentEl('ai-consent-body')?.textContent).toContain('https://llm.example/v1')
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual([['openai', 'https://llm.example/v1']])
  })

  test('同意済みなら、キーを保存しても同意の画面は出さない', async () => {
    const { pinia, store } = setup({ apiKeyProviders: [] })
    store.setAIApiKey = async () => {}
    const w = mountAI(pinia)
    await w.get('input[type="password"]').setValue('sk-x')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
  })

  test('「キーを削除」は確認のモーダル（プロバイダ名つき）を経由し、確認するまで削除しない', async () => {
    const { pinia, store } = setup()
    const cleared: string[] = []
    store.clearAIApiKey = async (p: string) => {
      cleared.push(p)
    }
    const w = mountAI(pinia)
    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain(
      'の API キーを削除しますか？',
    )
    dialogButton('キャンセル')?.click()
    await flushPromises()
    expect(cleared).toEqual([])
    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    dialogButton('削除する')?.click()
    await flushPromises()
    expect(cleared).toEqual(['openai'])
  })

  test('同意のボタンから同意の画面を開いて同意できる（Base URL が未保存の間は押せない）', async () => {
    const { pinia, store } = setup({ consentedProviders: [] })
    const agreed: string[][] = []
    store.recordAIConsent = async (p: string, url = '') => {
      agreed.push([p, url])
    }
    const w = mountAI(pinia)
    await w.get('input[type="url"]').setValue('https://draft.example/v1')
    expect(w.get('[data-testid="ai-consent-now"]').attributes('disabled')).toBeDefined()
    await w.get('input[type="url"]').setValue('')
    await w.get('[data-testid="ai-consent-now"]').trigger('click')
    expect(consentEl('ai-consent-body')?.textContent).toContain(
      'OpenAI 互換（https://api.openai.com）',
    )
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual([['openai', '']])
  })
})

describe('AISettings: 「ログイン」「API キーを追加」から来たとき（ui.aiConnectStart）', () => {
  test('ChatGPT・未同意: 同意の画面が出て、同意するとログインを始める', async () => {
    const { pinia, store } = setup({ ...chatgptCfg([]), consentedProviders: [] })
    const ids: string[] = []
    store.recordAIConsent = async (p: string) => {
      store.aiConfig = { ...base, consentedProviders: [p] }
    }
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
    }
    useUiStore().aiConnectStart = true
    mountAI(pinia, 'chatgpt')
    await flushPromises()
    expect(consentEl('ai-consent-body')).not.toBeNull()
    expect(ids).toEqual([])
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(ids).toEqual([''])
    expect(useUiStore().aiConnectStart).toBe(false)
  })

  test('ChatGPT・同意済み: 同意の画面を出さず、そのままログインを始める', async () => {
    const { pinia, store } = setup({ ...chatgptCfg([]), consentedProviders: ['chatgpt'] })
    const ids: string[] = []
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
    }
    useUiStore().aiConnectStart = true
    mountAI(pinia, 'chatgpt')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(ids).toEqual([''])
  })

  test('開いただけ（「設定」から）では、何も始めない', async () => {
    const { pinia, store } = setup({ ...chatgptCfg([]), consentedProviders: [] })
    store.signInChatGPT = async () => {
      throw new Error('始めてはいけない')
    }
    mountAI(pinia, 'chatgpt')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
  })

  test('API キーのプロバイダ・未同意: 同意の画面を出し、同意のあとキーの入力欄へ進む', async () => {
    const { pinia, store } = setup({ apiKeyProviders: [], consentedProviders: [] })
    store.recordAIConsent = async (p: string) => {
      store.aiConfig = { ...base, consentedProviders: [p] }
    }
    useUiStore().aiConnectStart = true
    const w = mountAI(pinia, 'anthropic')
    await flushPromises()
    expect(consentEl('ai-consent-body')?.textContent).toContain('Anthropic')
    consentEl('ai-consent-agree')?.click()
    await until(() => document.activeElement === w.get('input[type="password"]').element)
    expect(document.activeElement).toBe(w.get('input[type="password"]').element)
  })

  test('OpenAI 互換: 同意の前に、まず Base URL の入力へ進める', async () => {
    const { pinia } = setup({ apiKeyProviders: [], consentedProviders: [] })
    useUiStore().aiConnectStart = true
    const w = mountAI(pinia, 'openai')
    await flushPromises()
    expect(consentEl('ai-consent-body')).toBeNull()
    expect(document.activeElement).toBe(w.get('input[type="url"]').element)
  })
})

describe('AISettings Continue with ChatGPT（#75）', () => {
  test('API キー・Base URL の欄を出さず、ログインを案内する。未ログインは Continue with ChatGPT', () => {
    const { pinia } = setup({ ...chatgptCfg([]) })
    const w = mountAI(pinia, 'chatgpt')
    expect(w.find('input[type="password"]').exists()).toBe(false)
    expect(w.find('input[type="url"]').exists()).toBe(false)
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('未ログイン')
    expect(w.get('[data-testid="ai-chatgpt-login"]').text()).toBe('Continue with ChatGPT')
    expect(w.get('[data-testid="ai-destination"]').text()).toContain('https://api.openai.com')
    expect(w.text()).toContain('パスワードも Cookie も受け取りません')
    expect(w.text()).toContain('OpenAI の公式アプリではありません')
    expect(w.find('[data-testid="ai-chatgpt-accounts"]').exists()).toBe(false)
  })

  test('ログインを押すとブラウザでのログインを待つ。完了するとメールを出し、Using ChatGPT plan／Manage usage の組は出さない', async () => {
    const { pinia, store } = setup({ ...chatgptCfg([]) })
    let release: () => void = () => {}
    const ids: string[] = []
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
      await new Promise<void>((r) => {
        release = r
      })
      store.aiConfig = { ...base, ...chatgptCfg([acctA]) }
    }
    const w = mountAI(pinia, 'chatgpt')
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    expect(ids).toEqual([''])
    expect(w.text()).toContain('ブラウザで ChatGPT のログインを完了してください')
    expect(w.find('.waiting .spinner').exists()).toBe(true)
    expect(w.text()).toContain('ログインを中止')
    release()
    await flushPromises()
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('a@chatgpt.example')
    expect(w.find('.waiting').exists()).toBe(false)
    expect(w.text()).not.toContain('Using ChatGPT plan')
    expect(w.text()).not.toContain('Manage usage')
  })

  test('ログアウトでき、OpenAI 側の失効を確認できなかったときはそれを知らせる', async () => {
    const { pinia, store } = setup(chatgptCfg([acctA]))
    const ids: string[] = []
    store.signOutChatGPT = async (id: string) => {
      ids.push(id)
      store.aiConfig = { ...base, ...chatgptCfg([{ ...acctA, signedIn: false }]) }
      return false
    }
    const w = mountAI(pinia, 'chatgpt')
    await w.get('[data-testid="ai-chatgpt-logout"]').trigger('click')
    await flushPromises()
    dialogButton('ログアウトする')?.click()
    await flushPromises()
    expect(ids).toEqual(['oaiapp_AAAA'])
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('未ログイン')
  })

  test('ログインの失敗は理由を表示し、中止はエラーにしない', async () => {
    const { pinia, store } = setup({ ...chatgptCfg([]) })
    store.signInChatGPT = async () => {
      throw {
        code: 'OAUTH_DENIED',
        message: 'ChatGPT のログインが許可されませんでした。',
        retryable: false,
      }
    }
    const w = mountAI(pinia, 'chatgpt')
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('許可されませんでした')

    store.signInChatGPT = async () => {
      throw { code: 'OAUTH_CANCELLED', message: 'キャンセルしました。', retryable: false }
    }
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })
})

describe('AISettings の見出しと説明文の組（#241）', () => {
  test('見出しと直後の説明文・状態文は .group に束ね、組どうしは .group の外の親の間隔で離す', () => {
    const { pinia } = setup({ apiKeyProviders: ['anthropic'] })
    const w = mountAI(pinia, 'chatgpt')
    const groups = w.findAll('.group')
    // AISettings の見出し＋intro、AIApiKey の見出し＋状態、AIConsentSection の見出し＋状態
    expect(groups.length).toBeGreaterThanOrEqual(3)
    for (const g of groups) {
      expect(g.find('h3, h4').exists()).toBe(true)
      expect(g.findAll('p').length).toBeGreaterThanOrEqual(1)
    }
  })
})
