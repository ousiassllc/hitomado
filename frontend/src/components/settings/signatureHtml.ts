// 署名 HTML の保存用正規化。エディタは空でも `<p></p>` を出すので、テキストが無ければ空文字（署名なし）にする。
export function normalizeSignatureHtml(html: string): string {
  const text = html
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .trim()
  return text === '' ? '' : html
}
