// Ousiass デザインガイド準拠の回帰テスト。トークンの値（C6・#178）を tokens.css から確かめる。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { contrastRatio } from '../../src/lib/contrast'

const css = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8')
const darkStart = css.indexOf('@media (prefers-color-scheme: dark)')
const darkEnd = css.indexOf('@media (prefers-reduced-motion')
if (darkStart < 0 || darkEnd <= darkStart)
  throw new Error('tokens.css のダーク区間を見つけられない')

function vars(block: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of block.matchAll(/(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{3,6})\s*;/g)) out[m[1]] = m[2]
  return out
}

const light = vars(css.slice(0, darkStart))
const darkOwn = vars(css.slice(darkStart, darkEnd))
const dark = { ...light, ...darkOwn }

// ガイド C6: --mute はライト #6b6b6e・ダーク #8b8b8e の別値で、どちらも --bone・--surface 上で AA を満たす。
describe('--mute の文字色コントラスト（WCAG AA 4.5:1・C6）', () => {
  test('ライトとダークで別値にし、ダークのブロックが明示する（ライトの値を継がない）', () => {
    expect(light['--mute']).toBe('#6b6b6e')
    expect(darkOwn['--mute']).toBe('#8b8b8e')
  })

  const themes: [string, Record<string, string>][] = [
    ['ライト', light],
    ['ダーク', dark],
  ]
  for (const [theme, v] of themes) {
    for (const bg of ['--bone', '--surface']) {
      test(`${theme}: ${bg} 上で 4.5:1 以上`, () => {
        expect(contrastRatio(v['--mute'], v[bg])).toBeGreaterThanOrEqual(4.5)
      })
    }
  }
})
