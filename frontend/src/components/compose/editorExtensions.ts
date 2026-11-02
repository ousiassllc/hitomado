// 作成エディタの Tiptap 拡張。許可する書式は太字・リンク・箇条書き（順序なし）のみ（F-11）。
// 署名ブロック（data-hitomado-signature）と引用ブロック（data-hitomado-quote）は専用ノードとして属性ごと保持する。

import StarterKit from '@tiptap/starter-kit'
import { type AnyExtension, Extension, mergeAttributes, Node } from '@tiptap/vue-3'
import { isAllowedLink } from '../../lib/richtext'
import { ACCOUNT_ATTR, QUOTE_ATTR, SIGNATURE_ATTR } from '../../lib/signature'

/** `<div data-hitomado-signature data-account-id="…">`。内容（-- 区切り＋署名）は編集可能。 */
export const SignatureBlock = Node.create({
  name: 'signatureBlock',
  group: 'block',
  content: 'block+',
  defining: true,
  isolating: true,
  addAttributes() {
    return {
      accountId: {
        default: '',
        parseHTML: (el) => el.getAttribute(ACCOUNT_ATTR) ?? '',
        renderHTML: (attrs) => ({ [ACCOUNT_ATTR]: attrs.accountId as string }),
      },
    }
  },
  parseHTML() {
    return [{ tag: `div[${SIGNATURE_ATTR}]`, priority: 60 }]
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes({ [SIGNATURE_ATTR]: '' }, HTMLAttributes), 0]
  },
})

/** `<blockquote data-hitomado-quote>`。返信・転送の引用。 */
export const QuoteBlock = Node.create({
  name: 'quoteBlock',
  group: 'block',
  content: 'block+',
  defining: true,
  isolating: true,
  parseHTML() {
    return [{ tag: `blockquote[${QUOTE_ATTR}]`, priority: 60 }]
  },
  renderHTML({ HTMLAttributes }) {
    return ['blockquote', mergeAttributes({ [QUOTE_ATTR]: '' }, HTMLAttributes), 0]
  },
})

/**
 * Ctrl/Cmd+Enter は送信確認を開くショートカット（ComposePane）。StarterKit の「Mod-Enter で改行」を先に消費して、
 * 本文に余計な改行を入れない（キーイベント自体は上位へ伝わる）。
 */
const SendShortcutGuard = Extension.create({
  name: 'sendShortcutGuard',
  priority: 1000,
  addKeyboardShortcuts() {
    return { 'Mod-Enter': () => true }
  },
})

/** 許可外の書式（見出し・コード・取り消し線・斜体・下線・番号付きリスト・水平線・引用の素の blockquote）を外した拡張一式。 */
export function editorExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      blockquote: false,
      code: false,
      codeBlock: false,
      heading: false,
      horizontalRule: false,
      italic: false,
      orderedList: false,
      strike: false,
      underline: false,
      trailingNode: false,
      link: {
        openOnClick: false,
        autolink: true,
        linkOnPaste: true,
        protocols: ['http', 'https', 'mailto'],
        isAllowedUri: (url) => isAllowedLink(url),
        HTMLAttributes: { target: null, rel: null, class: null },
      },
    }),
    SendShortcutGuard,
    SignatureBlock,
    QuoteBlock,
  ]
}
