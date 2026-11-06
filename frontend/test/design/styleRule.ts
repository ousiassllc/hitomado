import { expect } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'

/** コンポーネントの <style> から、セレクタ（`.x {` まで含めて指定）の宣言ブロックを返す。 */
export function rule(file: string, selector: string): string {
  const style = parse(
    readFileSync(new URL(`../../src/components/${file}`, import.meta.url), 'utf8'),
  ).descriptor.styles[0].content
  const at = style.indexOf(selector)
  expect(at).toBeGreaterThanOrEqual(0)
  return style.slice(at).match(/\{([^}]*)\}/)?.[1] ?? ''
}
