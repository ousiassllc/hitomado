// 1 画面ずつのレイアウト（狭幅）のスレッド画面: 戻るボタン、返信の埋め込みを出さないこと。
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  ctl,
  detail,
  installMocks,
  REF,
  resetState,
  restoreMocks,
} from '../../src/components/thread/threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../src/stores/thread')
const { useInboxStore } = await import('../../src/stores/inbox')
const { usePlatformStore } = await import('../../src/stores/platform')
const { useLayoutStore } = await import('../../src/stores/layout')
const { default: ThreadPane } = await import('../../src/components/thread/ThreadPane.vue')

const original = window.matchMedia
/** 幅 width の matchMedia を差し込んで、レイアウトの監視を始める。 */
function startAt(width: number, touchUi: boolean) {
  window.matchMedia = ((q: string) => ({
    matches: width >= Number(/min-width: (\d+)px/.exec(q)?.[1]),
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia
  usePlatformStore().capabilities = { ...usePlatformStore().capabilities, touchUi }
  useLayoutStore().start()
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})
afterEach(() => {
  useLayoutStore().dispose()
  window.matchMedia = original
  document.documentElement.removeAttribute('data-touch-ui')
  delete document.documentElement.dataset.layout
})

async function openThread() {
  ctl.responses.GetThread = detail(false)
  const w = mount(ThreadPane)
  await useThreadStore().open(REF)
  await w.vm.$nextTick()
  return w
}

describe('ThreadPane（狭幅）', () => {
  test('1 画面ずつでは件名の横に「一覧に戻る」ボタンが出て、押すと開いているスレッドを閉じる', async () => {
    startAt(390, true)
    const inbox = useInboxStore()
    inbox.select(REF)
    const w = await openThread()
    const back = w.get('[data-testid="back-to-list"]')
    expect(back.attributes('aria-label')).toBe('一覧に戻る')
    await back.trigger('click')
    expect(inbox.selectedKey).toBeUndefined()
  })

  test('3 ペイン（広い・Windows）では戻るボタンを出さない', async () => {
    startAt(1440, false)
    const w = await openThread()
    expect(w.find('[data-testid="back-to-list"]').exists()).toBe(false)
  })

  test('幅が狭くても、タッチ UI でない OS（Windows）では戻るボタンを出さない', async () => {
    startAt(390, false)
    const w = await openThread()
    expect(w.find('[data-testid="back-to-list"]').exists()).toBe(false)
  })

  test('2 ペイン（中くらいの幅）では戻るボタンを出さない（一覧が横に見えている）', async () => {
    startAt(800, true)
    const w = await openThread()
    expect(w.find('[data-testid="back-to-list"]').exists()).toBe(false)
  })
})
