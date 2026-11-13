import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AccountItem from './AccountItem.vue'

function mountItem(needsReauth: boolean) {
  setActivePinia(createPinia())
  return mount(AccountItem, {
    props: {
      account: { id: 'a1', displayName: '仕事', email: 'work@example.com', color: 'blue' },
      selected: false,
      unread: 0,
      needsReauth,
    },
  })
}

describe('AccountItem の「要再接続」ボタン', () => {
  test('BaseIconButton（icon-btn・ghost）で出し、ラベルと再接続の通知を持つ', async () => {
    const w = mountItem(true)
    const btn = w.get('button.warn')
    expect(btn.classes()).toContain('icon-btn')
    expect(btn.classes()).toContain('icon-btn-ghost')
    expect(btn.classes()).not.toContain('icon-btn-danger')
    expect(btn.find('.warn-icon').exists()).toBe(true)
    expect(btn.attributes('aria-label')).toBe('仕事: 再接続が必要です（再接続）')
    await btn.trigger('click')
    expect(w.emitted('reconnect')).toHaveLength(1)
    w.unmount()
  })

  test('再接続が要らなければ出さない', () => {
    const w = mountItem(false)
    expect(w.find('button.warn').exists()).toBe(false)
    w.unmount()
  })
})

describe('AccountItem の識別色（#347）', () => {
  test('左の識別帯は無く、名前は一覧と同じ薄いチップ（識別色は tint）で出す。文言はそのまま', () => {
    const w = mountItem(false)
    expect(w.find('.band').exists()).toBe(false)
    const chip = w.get('.account-chip')
    expect(chip.classes()).toContain('tinted')
    expect(chip.attributes('style')).toContain('--chip-tint: var(--account-blue)')
    expect(chip.text()).toBe('仕事')
    expect(w.get('.email').text()).toBe('work@example.com')
    w.unmount()
  })
})
