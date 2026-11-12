import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useInboxStore } from '../../stores/inbox'
import { useInboxZeroStore } from '../../stores/inboxZero'
import { useSettingsStore } from '../../stores/settings'
import InboxTabs from './InboxTabs.vue'
import InboxZeroDialog from './InboxZeroDialog.vue'

function setup(opts: { items?: number; keyed?: boolean } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const inbox = useInboxStore()
  inbox.items = Array.from({ length: opts.items ?? 2 }, (_, i) => ({
    accountId: 'a1',
    threadId: `t${i}`,
    subject: 's',
    snippet: '',
    lastMessageAt: 1,
    lastFrom: { email: 'x@example.com' },
    messageCount: 1,
    isUnread: true,
    hasAttachment: false,
    inInbox: true,
    labels: null,
  }))
  const settings = useSettingsStore()
  settings.aiConfig = {
    provider: 'chatgpt',
    model: '',
    configured: false,
    hasApiKey: false,
    apiKeyProviders: opts.keyed === false ? [] : ['typesafe'],
    consentedProviders: ['typesafe'],
    chatgptSignedIn: false,
    chatgptAccounts: [],
  }
  return { pinia, zero: useInboxZeroStore() }
}

const LABEL = 'Inbox Zero：不要なメールをまとめてアーカイブ'

describe('Inbox Zero ボタン（#314）', () => {
  test('「すべて」タブの左に、ラベルなし（アイコンだけ）で置く。aria-label は仕様の文言で、title 属性は使わない', () => {
    const { pinia } = setup()
    const w = mount(InboxTabs, { props: { modelValue: 'all' }, global: { plugins: [pinia] } })
    const btn = w.get('[data-testid="inbox-zero"]')
    expect(btn.attributes('aria-label')).toBe(LABEL)
    expect(btn.text()).toBe('')
    expect(btn.find('svg').exists()).toBe(true)
    expect(btn.attributes('title')).toBeUndefined()
    expect(btn.attributes('aria-disabled')).toBeUndefined()
    // DOM の並び: ボタンが先頭のタブ「すべて」より前
    const html = w.html()
    expect(html.indexOf('data-testid="inbox-zero"')).toBeLessThan(
      html.indexOf('data-testid="inbox-tab-all"'),
    )
    // タブの横スクロールの外（tablist の中に入っていない）
    expect(w.get('[data-testid="inbox-tabs"]').find('[data-testid="inbox-zero"]').exists()).toBe(
      false,
    )
    w.unmount()
  })

  test('押せないとき（一覧が空・キー未登録）は aria-disabled で理由を aria-label に含め、押すと理由のトーストだけで判断は始めない', async () => {
    const { pinia, zero } = setup({ keyed: false })
    const w = mount(InboxTabs, { props: { modelValue: 'all' }, global: { plugins: [pinia] } })
    const btn = w.get('[data-testid="inbox-zero"]')
    expect(btn.attributes('aria-disabled')).toBe('true')
    expect(btn.attributes('aria-label')).toContain('API キーが未登録')
    expect(btn.attributes('disabled')).toBeUndefined() // 本物の disabled にするとツールチップ・タップが届かない
    await btn.trigger('click')
    expect(zero.phase).toBe('idle')
    w.unmount()
  })

  test('判断中はスピナーと aria-busy を出し、押しても二度目の判断は始まらない（やめる操作は判断中のダイアログ）', async () => {
    const { pinia, zero } = setup()
    const w = mount(InboxTabs, { props: { modelValue: 'all' }, global: { plugins: [pinia] } })
    zero.phase = 'checking'
    await w.vm.$nextTick()
    const btn = w.get('[data-testid="inbox-zero"]')
    expect(btn.attributes('aria-busy')).toBe('true')
    expect(btn.find('[role="status"]').exists()).toBe(true)
    expect(btn.find('svg').exists()).toBe(false)
    w.unmount()
  })
})

describe('Inbox Zero の確認の一覧（#314）', () => {
  const cands = [
    {
      ref: { accountId: 'a1', threadId: 't1' },
      from: '広告社',
      subject: '半額セール',
      reason: 'ad',
    },
    {
      ref: { accountId: 'a1', threadId: 't2' },
      from: 'GitHub',
      subject: '[repo] CI 通知',
      reason: 'notification',
    },
  ]

  function openReview() {
    const ctx = setup()
    ctx.zero.candidates = cands
    ctx.zero.checked = new Set(cands.map((c) => `${c.ref.accountId}:${c.ref.threadId}`))
    ctx.zero.phase = 'review'
    const w = mount(InboxZeroDialog, {
      props: {},
      global: { plugins: [ctx.pinia] },
      attachTo: document.body,
    })
    return { ...ctx, w }
  }

  test('差出人と件名だけをチェックボックスつきで出し、最初は全部チェック済み。Yes／No のボタンがある', async () => {
    document.body.innerHTML = ''
    const { w } = openReview()
    await w.vm.$nextTick()
    const dlg = document.body
    const boxes = dlg.querySelectorAll<HTMLInputElement>(
      '[data-testid="inbox-zero-list"] input[type="checkbox"]',
    )
    expect(boxes).toHaveLength(2)
    expect([...boxes].every((b) => b.checked)).toBe(true)
    const list = dlg.querySelector('[data-testid="inbox-zero-list"]')?.textContent ?? ''
    expect(list).toContain('広告社')
    expect(list).toContain('半額セール')
    expect(list).toContain('GitHub')
    expect(dlg.querySelector('[data-testid="inbox-zero-yes"]')?.textContent).toBe(
      'アーカイブする（Yes）',
    )
    expect(dlg.querySelector('[data-testid="inbox-zero-no"]')?.textContent).toBe('やめる（No）')
    expect(dlg.querySelector('[data-testid="inbox-zero-count"]')?.textContent).toContain(
      '2 件中 2 件を選択中',
    )
    w.unmount()
  })

  test('チェックを外すと選択数が減り、すべて外すと Yes は押せない。No は何も変えずに閉じる', async () => {
    document.body.innerHTML = ''
    const { w, zero } = openReview()
    await w.vm.$nextTick()
    const box = (n: number) =>
      document.querySelector<HTMLInputElement>(`[data-testid="inbox-zero-item-a1:t${n}"]`)
    box(1)?.click()
    await w.vm.$nextTick()
    expect(document.querySelector('[data-testid="inbox-zero-count"]')?.textContent).toContain(
      '2 件中 1 件を選択中',
    )
    box(2)?.click()
    await w.vm.$nextTick()
    expect(document.querySelector('[data-testid="inbox-zero-yes"]')?.hasAttribute('disabled')).toBe(
      true,
    )

    document.querySelector<HTMLButtonElement>('[data-testid="inbox-zero-no"]')?.click()
    await w.vm.$nextTick()
    expect(zero.phase).toBe('idle')
    expect(zero.candidates).toHaveLength(0)
    w.unmount()
  })
})

describe('Inbox Zero の判断中のダイアログ（#343）', () => {
  function openChecking() {
    document.body.innerHTML = ''
    const ctx = setup()
    ctx.zero.phase = 'checking'
    const w = mount(InboxZeroDialog, {
      props: {},
      global: { plugins: [ctx.pinia] },
      attachTo: document.body,
    })
    return { ...ctx, w }
  }

  test('判断中は、判断中の表示と「やめる」ボタンを持つダイアログが開く。判断中でなければ開かない', async () => {
    const { w } = openChecking()
    await w.vm.$nextTick()
    expect(document.querySelector('[data-testid="inbox-zero-checking"]')?.textContent).toContain(
      '不要なメールを探しています',
    )
    expect(document.querySelector('[data-testid="inbox-zero-checking-cancel"]')?.textContent).toBe(
      'やめる',
    )
    w.unmount()

    document.body.innerHTML = ''
    const idle = setup()
    const w2 = mount(InboxZeroDialog, {
      props: {},
      global: { plugins: [idle.pinia] },
      attachTo: document.body,
    })
    await w2.vm.$nextTick()
    expect(document.querySelector('[data-testid="inbox-zero-checking"]')).toBeNull()
    w2.unmount()
  })

  test('「やめる」で判断を中止して idle に戻る（phase が checking のときだけ中止の合図が走る）', async () => {
    const { w, zero } = openChecking()
    await w.vm.$nextTick()
    document.querySelector<HTMLButtonElement>('[data-testid="inbox-zero-checking-cancel"]')?.click()
    await w.vm.$nextTick()
    expect(zero.phase).toBe('idle')
    w.unmount()
  })

  test('Esc（Android の戻るは Esc として届く）でも判断を中止する', async () => {
    const { w, zero } = openChecking()
    await w.vm.$nextTick()
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    await w.vm.$nextTick()
    expect(zero.phase).toBe('idle')
    w.unmount()
  })
})
