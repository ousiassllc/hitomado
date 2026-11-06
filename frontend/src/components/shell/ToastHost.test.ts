import { describe, expect, test } from 'bun:test'

// docs/components/overview.md §3.4 の ToastHost（O3）・§5.3 の角丸（S1）と ToastHost.vue のスタイルを揃えておく。
const sfc = await Bun.file(new URL('./ToastHost.vue', import.meta.url)).text()

describe('ToastHost.vue', () => {
  test('「元に戻す」の角丸はガイドのボタンと同じ --r-md', () => {
    expect(sfc).toMatch(/\.undo\s*\{[^}]*border-radius:\s*var\(--r-md\);/)
  })

  test('トーストの置き換えは前のものが消えてから出し、表示時間は入場が終わってから数える（O3: 同時に 1 つ・3 秒の実表示）', () => {
    expect(sfc).toMatch(
      /<Transition\s+name="toast"\s+mode="out-in"\s+appear\s+@after-enter="startTimer">/,
    )
  })

  test('本文は .msg に出し、効かない title 属性で二重に読ませない（O3）', () => {
    expect(sfc).toContain('<span class="msg">{{ ui.toast.message }}</span>')
    expect(sfc).not.toMatch(/:title=/)
  })
})
