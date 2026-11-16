import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RecipientChip from './RecipientChip.vue'

// docs/components/overview.md §5.3（O4: z-index は --z-* のトークン）。× のリップルの面は z-index でなく DOM 順で前に出す（touch.css の .chip-remove::before が先）。
const sfc = await Bun.file(new URL('./RecipientChip.vue', import.meta.url)).text()

describe('RecipientChip.vue', () => {
  test('z-index の数値を直書きしない', () => {
    expect(sfc).not.toMatch(/z-index/)
  })

  // M4: touch.css の ::before（拡張した当たり判定）の押下は button 本体が受けるので、内側の円（v-ripple の面）へ転送する。
  test('当たり判定の外周（button 本体）の押下でも内側の面にリップルを出す', () => {
    expect(sfc).toMatch(/<button[^>]*@pointerdown\.self="rippleFromHitArea"/s)
    expect(sfc).toMatch(/ref="face"\s+v-ripple/)
    expect(sfc).toMatch(/spawn\(face\.value,\s*ev\)/)
  })

  test('形式が正しくないアドレスの理由は title でなくツールチップで出し、正しいアドレスには出さない', async () => {
    setActivePinia(createPinia())
    const bad = mount(RecipientChip, {
      props: { address: { email: 'bad' } },
      attachTo: document.body,
    })
    expect(bad.get('.chip').attributes('title')).toBeUndefined()
    await bad.get('.chip').trigger('focus')
    await flushPromises()
    expect(document.body.querySelector('.tip-pop')?.textContent).toContain(
      '形式が正しくないアドレスです',
    )
    bad.unmount()
    document.body.innerHTML = ''

    const ok = mount(RecipientChip, {
      props: { address: { email: 'a@example.com' } },
      attachTo: document.body,
    })
    await ok.get('.chip').trigger('focus')
    await flushPromises()
    expect(document.body.querySelector('.tip-pop')).toBeNull()
    ok.unmount()
  })
})
