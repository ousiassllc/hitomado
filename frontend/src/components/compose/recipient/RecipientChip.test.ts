import { describe, expect, test } from 'bun:test'

// docs/components/overview.md §5.3（O4: z-index は --z-* のトークン）。× のリップルの面は z-index でなく DOM 順で前に出す（touch.css の .chip-remove::before が先）。
const sfc = await Bun.file(new URL('./RecipientChip.vue', import.meta.url)).text()

describe('RecipientChip.vue', () => {
  test('z-index の数値を直書きしない', () => {
    expect(sfc).not.toMatch(/z-index/)
  })
})
