// BaseMenu の open 通知: メニューを開いた経路（ポインタ・Enter・Space・ArrowDown）によらず、ラベルを開いた時点で取得する（#338）。
import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  callsOf,
  ctl,
  detail,
  installMocks,
  REF,
  resetState,
  restoreMocks,
  until,
} from '../thread/threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../stores/thread')
const { default: ThreadPane } = await import('../thread/ThreadPane.vue')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})

// Reka UI のトリガーは ArrowDown でもメニューを開く。開いた経路によらず、開いた時点でラベルを取得する（#338）。
describe('ThreadHeader: ラベルメニュー', () => {
  const opens: [string, KeyboardEventInit | null][] = [
    ['ArrowDown', { key: 'ArrowDown' }],
    ['Enter', { key: 'Enter' }],
    ['Space', { key: ' ' }],
    ['クリック', null],
  ]
  for (const [name, key] of opens) {
    test(`${name} で開くと、ラベルを取得して並べる（「ラベルがありません」を出さない）`, async () => {
      document.body.innerHTML = ''
      ctl.responses.GetThread = detail(false)
      const w = mount(ThreadPane, { attachTo: document.body })
      await useThreadStore().open(REF)
      await flushPromises()
      expect(callsOf('ListLabels')).toHaveLength(0)
      const trigger = w.get('[data-testid="act-label"]')
      if (key)
        trigger.element.dispatchEvent(
          new KeyboardEvent('keydown', { ...key, bubbles: true, cancelable: true }),
        )
      else await trigger.trigger('click', { button: 0, ctrlKey: false })
      await until(() => document.querySelector('[role="menuitemcheckbox"]') !== null)
      expect(callsOf('ListLabels')).toHaveLength(1)
      expect(document.querySelector('[role="menu"]')?.textContent).toContain('仕事')
      expect(document.body.textContent).not.toContain('ラベルがありません')
      w.unmount()
    })
  }
})
