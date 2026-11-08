import { describe, expect, test } from 'bun:test'

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
})
