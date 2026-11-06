import { describe, expect, test } from 'bun:test'

// Ousiass デザインガイドのトークンで描くスクロールバー（#208）。値は scrollbar.css から確かめる。
const css = await Bun.file(new URL('./scrollbar.css', import.meta.url)).text()
const app = await Bun.file(new URL('../App.vue', import.meta.url)).text()

describe('scrollbar.css', () => {
  test(':root に細いスクロールバーを指定する（継承で全スクロール領域に効く）', () => {
    expect(css).toMatch(/:root\s*\{[^}]*scrollbar-width:\s*thin;/)
  })

  test('つまみ・溝の色はトークンで指定し、色値は直書きしない（ライト／ダークはトークン側が切り替える）', () => {
    expect(css).toMatch(/:root\s*\{[^}]*scrollbar-color:\s*var\(--mute\)\s+transparent;/)
    expect(css.replace(/\/\*[\s\S]*?\*\//g, '')).not.toMatch(/#[0-9a-fA-F]{3,6}\b/)
  })

  test('App.vue が読み込む', () => {
    expect(app).toContain("import './styles/scrollbar.css'")
  })
})
