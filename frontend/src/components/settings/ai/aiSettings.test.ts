import { beforeEach, describe, expect, test } from 'bun:test'
import BaseSelect from '../../ui/BaseSelect.vue'
import AIModelField from './AIModelField.vue'
import {
  acctA,
  base,
  button,
  chatgptCfg,
  consentEl,
  flushPromises,
  mountAI,
  setup,
} from './aiTestSupport'

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('AISettings モデル一覧（#76）', () => {
  test('取得した一覧があれば一覧から選ぶ。取得は勝手にしない（マウント時は呼ばない）', async () => {
    const { pinia, calls } = setup(
      {},
      {
        openai: {
          provider: 'openai',
          models: [{ id: 'gpt-a' }, { id: 'gpt-b', displayName: 'GPT B' }],
        },
      },
    )
    const w = mountAI(pinia)
    await flushPromises()
    expect(calls).toEqual([])
    const select = w.getComponent(AIModelField).getComponent(BaseSelect)
    expect(select.props('ariaLabel')).toBe('モデル')
    const opts = select.props('options') as { value: string; label: string }[]
    expect(opts.map((o) => o.value)).toContain('gpt-a')
    expect(opts.find((o) => o.value === 'gpt-b')?.label).toBe('GPT B（gpt-b）')
    expect(w.get('[data-testid="ai-models-note"]').text()).toContain('2 件')
  })

  test('保存済みのモデルが一覧に無くても消さず、選択肢に残す', () => {
    const { pinia } = setup({}, { openai: { provider: 'openai', models: [{ id: 'gpt-a' }] } })
    const w = mountAI(pinia)
    const all = w.getComponent(AIModelField).getComponent(BaseSelect).props('options') as {
      value: string
      label: string
    }[]
    expect(all[0]).toEqual({ value: 'gpt-old', label: 'gpt-old（一覧にありません）' })
  })

  test('一覧が取れないときは手入力のまま、理由を表示し、保存できる', async () => {
    const { pinia, store } = setup(
      { model: '' },
      {
        openai: {
          provider: 'openai',
          models: [],
          reason: 'unsupported',
          message:
            'このサービスにはモデル一覧の取得機能がないようです。モデル名を入力してください。',
        },
      },
    )
    let saved = ''
    store.setAIConfig = async (_p: string, m: string) => {
      saved = m
    }
    const w = mountAI(pinia)
    expect(w.find('[data-testid="ai-model"] input').exists()).toBe(true)
    expect(w.get('[data-testid="ai-models-note"]').text()).toContain('取得機能がない')
    await w.get('[data-testid="ai-model"] input').setValue('local-model')
    await button(w, '設定を保存').trigger('click')
    await flushPromises()
    expect(saved).toBe('local-model')
  })

  test('「一覧を更新」で取得する。API キーを保存したあとにも取得し、キーは入力欄から消える', async () => {
    const { pinia, store, calls } = setup()
    const keys: string[] = []
    store.setAIApiKey = async (_p: string, k: string) => {
      keys.push(k)
    }
    const w = mountAI(pinia)
    await button(w, '一覧を更新').trigger('click')
    await flushPromises()
    expect(calls).toEqual(['models:openai'])

    const key = w.get('input[type="password"]')
    await key.setValue('sk-secret-value')
    await button(w, 'キーを保存').trigger('click')
    await flushPromises()
    expect(keys).toEqual(['sk-secret-value'])
    expect(calls).toEqual(['models:openai', 'models:openai'])
    expect((w.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(w.html()).not.toContain('sk-secret-value')
  })

  test('「キーを削除」は確認のモーダル（プロバイダ名つき）を経由し、確認するまで削除しない', async () => {
    const { pinia, store } = setup()
    const cleared: string[] = []
    store.clearAIApiKey = async (p: string) => {
      cleared.push(p)
    }
    const w = mountAI(pinia)
    const click = (label: string) =>
      [...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')]
        .find((b) => b.textContent?.trim() === label)
        ?.click()
    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')?.textContent).toContain(
      'の API キーを削除しますか？',
    )
    click('キャンセル')
    await flushPromises()
    expect(cleared).toEqual([])
    await button(w, 'キーを削除').trigger('click')
    await flushPromises()
    click('削除する')
    await flushPromises()
    expect(cleared).toEqual(['openai'])
  })

  test('Base URL は保存前でも、決めたとき（Enter）にその送信先で取得する。入力しただけでは取得しない', async () => {
    const { pinia, store, loads } = setup()
    store.setAIConfig = async () => {}
    const w = mountAI(pinia)
    const url = w.get('input[type="url"]')
    await url.setValue('https://llm.example/v1')
    await flushPromises()
    expect(loads).toEqual([])
    expect(button(w, '一覧を更新').attributes('disabled')).toBeUndefined()
    expect(w.text()).not.toContain('先に「設定を保存」')
    await url.trigger('keydown.enter')
    await flushPromises()
    expect(loads).toEqual([{ provider: 'openai', baseUrl: 'https://llm.example/v1' }])
  })

  test('同意前の送信先は、同意の画面で同意すると一覧を取得する（設定画面の同意のボタンも同じ画面）', async () => {
    const { pinia, store, calls } = setup({ consentedProviders: [] })
    const agreed: string[][] = []
    store.recordAIConsent = async (p: string, url = '') => {
      agreed.push([p, url])
    }
    const w = mountAI(pinia)
    await w.get('[data-testid="ai-consent-now"]').trigger('click')
    expect(consentEl('ai-consent-body')?.textContent).toContain(
      'OpenAI 互換（https://api.openai.com）',
    )
    consentEl('ai-consent-agree')?.click()
    await flushPromises()
    expect(agreed).toEqual([['openai', '']])
    expect(calls).toEqual(['models:openai'])
  })
})

describe('AISettings Continue with ChatGPT（#75）', () => {
  test('API キー・Base URL の欄を出さず、ログインを案内する。未ログインは Continue with ChatGPT', () => {
    const { pinia } = setup({ provider: 'chatgpt', hasApiKey: false })
    const w = mountAI(pinia)
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
    const { pinia, store, calls } = setup({ provider: 'chatgpt', hasApiKey: false })
    let release: () => void = () => {}
    const ids: string[] = []
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
      await new Promise<void>((r) => {
        release = r
      })
      store.aiConfig = { ...base, ...chatgptCfg([acctA]) }
    }
    const w = mountAI(pinia)
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    expect(ids).toEqual([''])
    expect(w.text()).toContain('ブラウザで ChatGPT のログインを完了してください')
    expect(w.text()).toContain('ログインを中止')
    release()
    await flushPromises()
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('a@chatgpt.example')
    expect(w.text()).not.toContain('Using ChatGPT plan')
    expect(w.text()).not.toContain('Manage usage')
    expect(calls).toEqual(['models:chatgpt'])
  })

  test('ログアウトでき、OpenAI 側の失効を確認できなかったときはそれを知らせる', async () => {
    const { pinia, store } = setup(chatgptCfg([acctA]))
    const ids: string[] = []
    store.signOutChatGPT = async (id: string) => {
      ids.push(id)
      store.aiConfig = { ...base, ...chatgptCfg([{ ...acctA, signedIn: false }]) }
      return false
    }
    const w = mountAI(pinia)
    await w.get('[data-testid="ai-chatgpt-logout"]').trigger('click')
    await flushPromises()
    expect(ids).toEqual(['oaiapp_AAAA'])
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('未ログイン')
  })

  test('ログインの失敗は理由を表示し、中止はエラーにしない', async () => {
    const { pinia, store } = setup({ provider: 'chatgpt', hasApiKey: false })
    store.signInChatGPT = async () => {
      throw {
        code: 'OAUTH_DENIED',
        message: 'ChatGPT のログインが許可されませんでした。',
        retryable: false,
      }
    }
    const w = mountAI(pinia)
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
