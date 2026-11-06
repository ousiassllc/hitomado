import { describe, expect, test } from 'bun:test'

// docs/components/overview.md §3.4 の ToastHost（O3）・§5.3 の角丸（S1）と ToastHost.vue のスタイルを揃えておく。
const sfc = await Bun.file(new URL('./ToastHost.vue', import.meta.url)).text()

describe('ToastHost.vue', () => {
  test('「元に戻す」の角丸はガイドのボタンと同じ --r-md', () => {
    expect(sfc).toMatch(/\.undo\s*\{[^}]*border-radius:\s*var\(--r-md\);/)
  })

  test('トーストの置き換えは前のものが消えてから出す（O3: 同時に 1 つ。退出中のものと並ばない）', () => {
    expect(sfc).toMatch(/<Transition\s+name="toast"\s+mode="out-in">/)
  })
})
