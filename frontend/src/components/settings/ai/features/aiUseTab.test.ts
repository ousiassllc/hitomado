import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { features } from '../../../../../test/aiConfigKit'
import { useUiStore } from '../../../../stores/ui'
import BaseSelect from '../../../ui/BaseSelect.vue'
import { acctA, button, chatgptCfg, setup } from '../aiTestSupport'
import AIUseTab from './AIUseTab.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

const MODELS = {
  openai: { provider: 'openai', models: [{ id: 'gpt-a' }, { id: 'gpt-b' }] },
  chatgpt: { provider: 'chatgpt', models: [{ id: 'gpt-plan' }] },
  anthropic: {
    provider: 'anthropic',
    models: [{ id: 'claude-new', efforts: ['low', 'high'] }, { id: 'claude-old' }],
  },
}
const normal = {
  ...chatgptCfg([acctA]),
  apiKeyProviders: ['openai', 'anthropic'],
  consentedProviders: ['chatgpt', 'openai', 'anthropic'],
  features: features(
    { provider: 'openai', model: 'gpt-a' },
    { provider: 'chatgpt', model: 'gpt-plan' },
  ),
}

function open(cfg = {}, load?: (p: string) => unknown) {
  const s = setup(
    cfg,
    undefined,
    (p) => (load?.(p) ?? MODELS[p as keyof typeof MODELS] ?? { provider: p, models: [] }) as never,
  )
  const saved: unknown[][] = []
  s.store.setAIFeature = async (...a: unknown[]) => {
    saved.push(a)
  }
  const w = mount(AIUseTab, { global: { plugins: [s.pinia] }, attachTo: document.body })
  return { ...s, w, saved }
}
const feat = (w: VueWrapper, id: string) => w.get(`[data-testid="ai-feature-${id}"]`)
const selects = (w: VueWrapper, id: string) => feat(w, id).findAllComponents(BaseSelect)
const values = (sel: ReturnType<typeof selects>[number]) =>
  (sel.props('options') as { value: string }[]).map((o) => o.value)

describe('「機能ごとに使う AI」タブ（#376）', () => {
  test('機能ごとに 1 行（AI で下書き・Inbox Zero・エージェント）。エージェントは「準備中」で選べず保存もしない', async () => {
    const { w, saved } = open(normal)
    await flushPromises()
    expect(w.findAll('[data-testid="ai-feature-list"] > li')).toHaveLength(3)
    expect(w.text()).toContain('AI で下書き')
    expect(w.text()).toContain('Inbox Zero')
    const agent = w.get('[data-testid="ai-feature-agent"]')
    expect(agent.text()).toContain('準備中')
    expect(agent.attributes('aria-disabled')).toBe('true')
    expect(agent.find('button, [role="combobox"]').exists()).toBe(false)
    expect(saved).toEqual([])
  })

  test('マウント時に、つないであるプロバイダのモデル一覧だけを取得する', async () => {
    const { w, loads } = open(normal)
    await flushPromises()
    expect(loads.map((l) => l.provider).sort()).toEqual(['anthropic', 'chatgpt', 'openai'])
    w.unmount()
  })

  test('選べるのは、つないであるプロバイダのモデルだけ。選ぶとすぐ保存し、エフォートは持ち越さない', async () => {
    const { w, saved } = open(normal)
    await flushPromises()
    const [pick] = selects(w, 'draft')
    expect(values(pick!)).toEqual([
      'chatgpt|gpt-plan',
      'anthropic|claude-new',
      'anthropic|claude-old',
      'openai|gpt-a',
      'openai|gpt-b',
    ])
    expect(pick?.props('modelValue')).toBe('openai|gpt-a')
    pick?.vm.$emit('update:modelValue', 'anthropic|claude-new')
    await flushPromises()
    expect(saved).toEqual([['draft', 'anthropic', 'claude-new', '']])
  })

  test('エフォートは「AI で下書き」の行だけ。選んだモデルで選べる段階を出す（Inbox Zero には出さない）', async () => {
    const { w, saved } = open({
      ...normal,
      features: features(
        { provider: 'anthropic', model: 'claude-new', effort: 'high' },
        { provider: 'chatgpt', model: 'gpt-plan' },
      ),
    })
    await flushPromises()
    expect(feat(w, 'draft').find('[data-testid="ai-effort"]').exists()).toBe(true)
    expect(feat(w, 'inboxZero').find('[data-testid="ai-effort"]').exists()).toBe(false)
    const effort = feat(w, 'draft').get('[data-testid="ai-effort"]').getComponent(BaseSelect)
    expect(values(effort).slice(1)).toEqual(['low', 'high'])
    expect(effort.props('modelValue')).toBe('high')
    effort.vm.$emit('update:modelValue', 'low')
    await flushPromises()
    expect(saved).toEqual([['draft', 'anthropic', 'claude-new', 'low']])
  })

  test('モデルを変えて選択中のエフォートが使えなくなったら「モデルの既定」に戻す。使えるなら引き継ぐ', async () => {
    const cfg = {
      ...normal,
      features: features(
        { provider: 'anthropic', model: 'claude-new', effort: 'high' },
        { provider: 'chatgpt', model: 'gpt-plan' },
      ),
    }
    const { w, saved } = open(cfg)
    await flushPromises()
    const [pick] = selects(w, 'draft')
    pick?.vm.$emit('update:modelValue', 'anthropic|claude-old') // 段階なし
    await flushPromises()
    pick?.vm.$emit('update:modelValue', 'chatgpt|gpt-plan') // high は共通の段階にある
    await flushPromises()
    expect(saved).toEqual([
      ['draft', 'anthropic', 'claude-old', ''],
      ['draft', 'chatgpt', 'gpt-plan', 'high'],
    ])
  })

  test('Inbox Zero にも選べて、保存は機能ごとに別（draft は変わらない）。Jev は選択肢の 1 つ', async () => {
    const { w, saved } = open({
      ...normal,
      apiKeyProviders: ['openai', 'anthropic', 'typesafe'],
      consentedProviders: [...normal.consentedProviders, 'typesafe'],
    })
    await flushPromises()
    const [pick] = selects(w, 'inboxZero')
    expect(values(pick!)).toContain('typesafe|')
    pick?.vm.$emit('update:modelValue', 'typesafe|')
    await flushPromises()
    expect(saved).toEqual([['inboxZero', 'typesafe', '', '']])
  })

  test('保存に失敗したら理由を行に出す', async () => {
    const { w, store } = open(normal)
    store.setAIFeature = async () => {
      throw { code: 'VALIDATION', message: 'モデルを選んでください。', retryable: false }
    }
    await flushPromises()
    selects(w, 'draft')[0]?.vm.$emit('update:modelValue', 'openai|gpt-b')
    await flushPromises()
    expect(feat(w, 'draft').get('[role="alert"]').text()).toContain('モデルを選んでください')
  })
})

describe('「機能ごとに使う AI」の状態（#376）', () => {
  test('(2) 何もつないでいなければ、タブ全体にひとつだけ案内を出し、「AI をつなぐ」で「つなぎ先」タブへ', async () => {
    const { w } = open({
      apiKeyProviders: [],
      consentedProviders: [],
      features: features(),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    await flushPromises()
    expect(w.findAll('[data-testid="ai-use-empty"]')).toHaveLength(1)
    expect(w.text()).toContain('まず AI をつなぐと、ここで機能ごとに選べます')
    expect(w.find('[data-testid="ai-feature-list"]').exists()).toBe(false)
    await w.get('[data-testid="ai-use-connect"]').trigger('click')
    expect(useUiStore().aiTab).toBe('connect')
  })

  test('(2) 何もつないでいなくても、Inbox Zero の理由のオン・オフ（AI を使わない理由を含む）は出る', async () => {
    const { w } = open({
      apiKeyProviders: [],
      consentedProviders: [],
      features: features(),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    await flushPromises()
    expect(w.find('[data-testid="ai-use-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="inbox-zero-reason-calendar"]').exists()).toBe(true)
    expect(w.find('[data-testid="inbox-zero-reason-replied"]').exists()).toBe(true)
  })

  test('(3) その機能で使える AI が無い行は「使える AI がありません」＋「AI をつなぐ」。ほかの行はそのまま', async () => {
    const { w } = open({
      apiKeyProviders: ['typesafe'],
      consentedProviders: ['typesafe'],
      features: features(),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    await flushPromises()
    const draft = feat(w, 'draft')
    expect(draft.get('[data-testid="ai-feature-empty"]').text()).toContain('使える AI がありません')
    await draft.get('[data-testid="ai-feature-connect-draft"]').trigger('click')
    expect(useUiStore().aiTab).toBe('connect')
    expect(feat(w, 'inboxZero').find('[data-testid="ai-feature-empty"]').exists()).toBe(false)
  })

  test('Jev だけのときの Inbox Zero は「TypeSafe AI（固定）」。選べる AI がそれだけなので、その選択を保存する', async () => {
    const { w, saved } = open({
      apiKeyProviders: ['typesafe'],
      consentedProviders: ['typesafe'],
      features: features(),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    await flushPromises()
    expect(feat(w, 'inboxZero').get('[data-testid="ai-feature-fixed"]').text()).toBe(
      'TypeSafe AI（Jev）（固定）',
    )
    expect(saved).toEqual([['inboxZero', 'typesafe', '', '']])
  })

  test('Jev のほかにもつないであるときは、モデル一覧の取得中に Jev を保存しない（取得が終わるまで「固定」にもしない）', async () => {
    const s = setup({
      apiKeyProviders: ['openai', 'typesafe'],
      consentedProviders: ['openai', 'typesafe'],
      features: features({ provider: 'openai', model: 'gpt-a' }),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    const finish: (() => void)[] = []
    const load = s.store.loadAIModels
    s.store.loadAIModels = (p: string, b = '') =>
      new Promise((resolve) => {
        finish.push(() => resolve(load(p, b)))
      })
    const saved: unknown[][] = []
    s.store.setAIFeature = async (...a: unknown[]) => {
      saved.push(a)
    }
    const w = mount(AIUseTab, { global: { plugins: [s.pinia] }, attachTo: document.body })
    await flushPromises()
    expect(finish).toHaveLength(1) // 取得の途中
    expect(feat(w, 'inboxZero').find('[data-testid="ai-feature-fixed"]').exists()).toBe(false)
    expect(saved).toEqual([])
    for (const f of finish) f()
    await flushPromises()
    expect(feat(w, 'inboxZero').find('[data-testid="ai-feature-fixed"]').exists()).toBe(false)
    expect(saved).toEqual([])
    w.unmount()
  })

  test('(4) 選んでいた AI が外された・ログインが切れたときは、選択を残して「つなぎ直しが必要」と直す入口を出す。別の AI へは切り替えない', async () => {
    const cfg = {
      ...normal,
      apiKeyProviders: ['anthropic'],
      chatgptSignedIn: false,
      chatgptAccounts: [{ ...acctA, signedIn: false }],
    }
    const { w, saved } = open(cfg)
    await flushPromises()
    for (const id of ['draft', 'inboxZero']) {
      expect(feat(w, id).attributes('data-state')).toBe('reconnect')
      expect(feat(w, id).get('[data-testid="ai-feature-reconnect"]').text()).toBe(
        'つなぎ直しが必要',
      )
    }
    const draft = selects(w, 'draft')[0]
    expect(draft?.props('modelValue')).toBe('openai|gpt-a') // 選択はそのまま
    expect(saved).toEqual([]) // 勝手に保存し直さない
    expect(feat(w, 'draft').get('[data-testid="ai-feature-fix-draft"]').text()).toBe(
      'つなぎ先を直す',
    )
    expect(feat(w, 'inboxZero').get('[data-testid="ai-feature-fix-inboxZero"]').text()).toBe(
      'ログインし直す',
    )
    const ui = useUiStore()
    await feat(w, 'inboxZero').get('[data-testid="ai-feature-fix-inboxZero"]').trigger('click')
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(ui.aiTab).toBe('connect')
    expect(ui.aiConnectStart).toBe(true)
  })

  test('つなぎ直しが必要な行には、つないである別の AI のモデルも選択肢に出る（自分で選び直せる）', async () => {
    const cfg = { ...normal, apiKeyProviders: ['anthropic'] }
    const { w } = open(cfg)
    await flushPromises()
    const [pick] = selects(w, 'draft')
    expect(values(pick!)).toEqual(
      expect.arrayContaining(['chatgpt|gpt-plan', 'anthropic|claude-new', 'openai|gpt-a']),
    )
  })

  test('モデル一覧を取得できないプロバイダは、モデル名を手入力して保存できる（理由を表示）', async () => {
    const { w, saved } = open({ ...normal, features: features() }, (p) =>
      p === 'openai'
        ? {
            provider: p,
            models: [],
            reason: 'unsupported',
            message: 'このサービスにはモデル一覧の取得機能がないようです。',
          }
        : undefined,
    )
    await flushPromises()
    const form = feat(w, 'draft').get('[data-testid="ai-feature-manual-draft-openai"]')
    expect(form.text()).toContain('取得機能がない')
    await form.get('input').setValue('local-model')
    await button(feat(w, 'draft'), 'モデルを保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['draft', 'openai', 'local-model', '']])
  })

  test('一覧の取得が失敗（例外）しても、そのプロバイダは手入力になり、タブは壊れない', async () => {
    const { w, store } = open(normal)
    store.loadAIModels = async () => {
      throw { code: 'OFFLINE', message: '通信できません。', retryable: true }
    }
    store.aiModels = {}
    // 接続の変化で取り直す
    store.aiConfig = {
      ...store.aiConfig!,
      apiKeyProviders: ['openai', 'anthropic', 'typesafe'],
      consentedProviders: [...normal.consentedProviders, 'typesafe'],
    }
    await flushPromises()
    expect(w.find('[data-testid="ai-feature-draft"]').exists()).toBe(true)
  })
})
