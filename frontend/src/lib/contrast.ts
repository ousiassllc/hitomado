// WCAG 2.x の相対輝度とコントラスト比（NF §11: アカウント色は 4.5:1 以上）。

export interface Rgb {
  r: number
  g: number
  b: number
}

/** `#rgb` / `#rrggbb` を RGB（0–255）にする。解釈できなければ例外。 */
export function parseHex(hex: string): Rgb {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) throw new Error(`invalid hex color: ${hex}`)
  let h = m[1]
  if (h.length === 3) {
    h = [...h].map((c) => c + c).join('')
  }
  return {
    r: Number.parseInt(h.slice(0, 2), 16),
    g: Number.parseInt(h.slice(2, 4), 16),
    b: Number.parseInt(h.slice(4, 6), 16),
  }
}

function channel(v: number): number {
  const s = v / 255
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
}

/** WCAG の相対輝度（0〜1）。 */
export function relativeLuminance({ r, g, b }: Rgb): number {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

/** 2 色のコントラスト比（1〜21）。引数の順序は問わない。 */
export function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(parseHex(a))
  const lb = relativeLuminance(parseHex(b))
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}
