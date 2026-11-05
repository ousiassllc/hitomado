import { expect, test } from 'bun:test'
import { sanitizeEmailHtml } from './index'

// 回帰: DOMPurify の補正（test/domPurifyShim.ts）を import しないテストが、先に本文を描いても
// サニタイズが効く（補正は setup.ts の preload で必ず先に入る）。
// 補正が無いと全要素が「許可されないタグ」になり、<script> も <img src="https://…"> もそのまま残っていた。
test('補正を import しないテストでもサニタイズが効く', () => {
  const out = sanitizeEmailHtml(
    '<p>a</p><script>alert(1)</script><img src="https://img.example/a.png">',
  )
  expect(out).toContain('<p>a</p>')
  expect(out).not.toContain('<script')
  expect(out).not.toContain('img.example')
})
