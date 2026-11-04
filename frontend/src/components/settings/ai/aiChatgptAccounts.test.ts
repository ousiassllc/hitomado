import { describe, expect, test } from 'bun:test'
import { acctA, acctB, base, chatgptCfg, flushPromises, mountAI, setup } from './aiTestSupport'

describe('AISettings 複数の ChatGPT アカウント（#79）', () => {
  test('保存済みのアカウントを一覧に出し、使用中を示す。追加はアカウントを追加ボタン', () => {
    const { pinia } = setup(chatgptCfg([acctA, acctB]))
    const w = mountAI(pinia)
    const rows = w.findAll('[data-testid^="ai-chatgpt-account-"]')
    expect(rows.map((r) => r.text())).toEqual([
      expect.stringContaining('a@chatgpt.example'),
      expect.stringContaining('b@chatgpt.example'),
    ])
    expect(rows[0]?.text()).toContain('使用中')
    expect(rows[1]?.text()).not.toContain('使用中')
    const radios = w.findAll('input[type="radio"]')
    expect(radios.map((r) => (r.element as HTMLInputElement).checked)).toEqual([true, false])
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('a@chatgpt.example')
    expect(w.find('[data-testid="ai-chatgpt-login"]').exists()).toBe(false)
    expect(w.get('[data-testid="ai-chatgpt-add"]').text()).toBe('アカウントを追加')
  })

  test('同じメールアドレスの登録は、登録の ID の末尾で見分ける', () => {
    const { pinia } = setup(
      chatgptCfg([
        acctA,
        {
          id: 'oaiapp_ZZZZ',
          email: 'a@chatgpt.example',
          signedIn: true,
          active: false,
          planNoticePending: false,
        },
      ]),
    )
    const w = mountAI(pinia)
    const texts = w.findAll('[data-testid^="ai-chatgpt-account-"]').map((r) => r.text())
    expect(texts[0]).toContain('a@chatgpt.example（登録 …AAAA）')
    expect(texts[1]).toContain('a@chatgpt.example（登録 …ZZZZ）')
  })

  test('別のアカウントを選ぶと切り替わり、そのアカウントのモデル一覧を取り直す', async () => {
    const { pinia, store, calls } = setup(chatgptCfg([acctA, acctB]))
    const picked: string[] = []
    store.selectChatGPTAccount = async (id: string) => {
      picked.push(id)
      store.aiConfig = {
        ...base,
        ...chatgptCfg([
          { ...acctA, active: false },
          { ...acctB, active: true },
        ]),
      }
    }
    const w = mountAI(pinia)
    await w.findAll('input[type="radio"]')[1]?.setValue(true)
    await flushPromises()
    expect(picked).toEqual(['oaiapp_BBBB'])
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('b@chatgpt.example')
    expect(w.get('[data-testid="ai-chatgpt-active"]').element.closest('li')?.textContent).toContain(
      'b@chatgpt.example',
    )
    expect(calls).toEqual(['models:chatgpt'])
  })

  test('選んだアカウントだけログアウトする。もう片方は残る', async () => {
    const { pinia, store } = setup(chatgptCfg([acctA, acctB]))
    const out: string[] = []
    store.signOutChatGPT = async (id: string) => {
      out.push(id)
      store.aiConfig = { ...base, ...chatgptCfg([acctA, { ...acctB, signedIn: false }]) }
      return true
    }
    const w = mountAI(pinia)
    const row = w.get('[data-testid="ai-chatgpt-account-oaiapp_BBBB"]')
    await row.get('[data-testid="ai-chatgpt-logout"]').trigger('click')
    await flushPromises()
    expect(out).toEqual(['oaiapp_BBBB'])
    const rows = w.findAll('[data-testid^="ai-chatgpt-account-"]')
    expect(rows).toHaveLength(2)
    expect(rows[1]?.text()).toContain('ログアウト済み')
    expect(rows[0]?.find('[data-testid="ai-chatgpt-logout"]').exists()).toBe(true)
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('a@chatgpt.example')
  })

  test('ログアウト中の進行表示は、押した行のボタンだけに出る', async () => {
    const { pinia, store } = setup(chatgptCfg([acctA, acctB]))
    let finish: (revoked: boolean) => void = () => {}
    store.signOutChatGPT = () => new Promise<boolean>((resolve) => (finish = resolve))
    const w = mountAI(pinia)
    const busyOf = (id: string) =>
      w
        .get(`[data-testid="ai-chatgpt-account-${id}"] [data-testid="ai-chatgpt-logout"]`)
        .attributes('aria-busy')
    await w
      .get('[data-testid="ai-chatgpt-account-oaiapp_BBBB"] [data-testid="ai-chatgpt-logout"]')
      .trigger('click')
    expect(busyOf('oaiapp_BBBB')).toBe('true')
    expect(busyOf('oaiapp_AAAA')).toBeUndefined()
    finish(true)
    await flushPromises()
    expect(busyOf('oaiapp_BBBB')).toBeUndefined()
  })

  test('ログアウト済みの登録は、その登録で再ログインでき、削除もできる', async () => {
    const out = { ...acctB, signedIn: false }
    const { pinia, store } = setup(chatgptCfg([acctA, out]))
    const ids: string[] = []
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
      store.aiConfig = {
        ...base,
        ...chatgptCfg([
          { ...acctA, active: false },
          { ...acctB, active: true },
        ]),
      }
    }
    const removed: string[] = []
    store.removeChatGPTAccount = async (id: string) => {
      removed.push(id)
      store.aiConfig = { ...base, ...chatgptCfg([acctA]) }
    }
    const w = mountAI(pinia)
    const row = w.get('[data-testid="ai-chatgpt-account-oaiapp_BBBB"]')
    expect(row.find('input[type="radio"]').exists()).toBe(false) // ログアウト済みは選べない
    await row.get('[data-testid="ai-chatgpt-relogin"]').trigger('click')
    await flushPromises()
    expect(ids).toEqual(['oaiapp_BBBB'])

    store.aiConfig = { ...base, ...chatgptCfg([acctA, out]) }
    await flushPromises()
    await w.get('[data-testid="ai-chatgpt-remove"]').trigger('click')
    await flushPromises()
    expect(removed).toEqual(['oaiapp_BBBB'])
    expect(w.findAll('[data-testid^="ai-chatgpt-account-"]')).toHaveLength(1)
  })

  test('全員ログアウト済みなら Continue with ChatGPT（新しい登録）を出し、使うアカウントの選択を促さない', async () => {
    const { pinia, store } = setup(chatgptCfg([{ ...acctA, signedIn: false }]))
    const ids: string[] = []
    store.signInChatGPT = async (id: string) => {
      ids.push(id)
    }
    const w = mountAI(pinia)
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain('未ログイン')
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    expect(ids).toEqual([''])
  })

  test('使用中が無く、ほかにログイン中のアカウントがあるときは選択を促す', () => {
    const { pinia } = setup({
      ...chatgptCfg([{ ...acctA, active: false }, acctB]),
      chatgptSignedIn: false,
      hasApiKey: false,
    })
    const w = mountAI(pinia)
    expect(w.get('[data-testid="ai-chatgpt-status"]').text()).toContain(
      '使うアカウントを選んでください',
    )
  })

  test('切り替え・ログアウトの失敗は理由を表示する', async () => {
    const { pinia, store } = setup(chatgptCfg([acctA, acctB]))
    store.selectChatGPTAccount = async () => {
      throw {
        code: 'AI_NOT_CONFIGURED',
        message: 'ChatGPT のログインが切れました。',
        retryable: false,
      }
    }
    const w = mountAI(pinia)
    const radios = () =>
      w.findAll('input[type="radio"]').map((r) => (r.element as HTMLInputElement).checked)
    expect(radios()).toEqual([true, false])
    const pressed = w.findAll('input[type="radio"]')[1]?.element
    await w.findAll('input[type="radio"]')[1]?.setValue(true)
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('ログインが切れました')
    // 失敗したら、押したラジオが選択されたまま残らず、使用中のアカウントに戻る。
    expect(radios()).toEqual([true, false])
    // ラジオは作り直さない（作り直すと、キーボードで操作していたフォーカスが失われる）。
    expect(w.findAll('input[type="radio"]')[1]?.element).toBe(pressed)
  })

  test('ラジオの名前は「<アカウント> を使う」で、一覧にも名前がある', () => {
    const { pinia } = setup(chatgptCfg([acctA, acctB]))
    const w = mountAI(pinia)
    expect(w.findAll('input[type="radio"]').map((r) => r.attributes('aria-label'))).toEqual([
      'a@chatgpt.example を使う',
      'b@chatgpt.example を使う',
    ])
    expect(w.get('[data-testid="ai-chatgpt-accounts"] ul').attributes('aria-label')).toBe(
      'ChatGPT アカウント',
    )
  })
})
