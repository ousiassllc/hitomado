// 新規作成で送信元を切り替えたときの署名差し替え（F-07、components/overview.md §3.7）。composeStore の一部。
// 確認が必要な場合（署名部分を編集済み）は conflict に入れ、利用者の選択を resolve で受ける。
import { type Ref, ref } from 'vue'
import type { ComposeSession } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { applySignature, findSignature, replaceSignature, signatureContent } from '../lib/signature'

interface BodyHolder {
  bodyHtml: string
}

interface SignatureSwapDeps {
  session: Ref<ComposeSession | undefined>
  content: Ref<BodyHolder>
  accounts: { byId(id: string): { signatureHtml: string } | undefined }
}

/** 差し替えの確認待ち。 */
export interface SignatureConflict {
  accountId: string
}

export function useSignatureSwap(deps: SignatureSwapDeps) {
  const { session, content, accounts } = deps
  const conflict = ref<SignatureConflict>()
  /**
   * 直前に挿入した署名ブロックの内側 HTML。`undefined` = まだ一度も署名を入れていない（送信元未選択で開いた）、
   * `''` = 署名なしのアカウントだった。
   */
  let inserted: string | undefined

  function signatureOf(accountId: string): string {
    return accounts.byId(accountId)?.signatureHtml ?? ''
  }

  /** セッション開始直後に呼ぶ。OpenCompose が返した本文に入っている署名ブロックを「直前に挿入した署名」とする。 */
  function init(): void {
    conflict.value = undefined
    const found = findSignature(content.value.bodyHtml)
    inserted = found ? found.content : session.value?.accountId ? '' : undefined
  }

  function reset(): void {
    conflict.value = undefined
    inserted = undefined
  }

  function put(accountId: string): void {
    const sig = signatureOf(accountId)
    content.value.bodyHtml = applySignature(content.value.bodyHtml, accountId, sig)
    inserted = signatureContent(sig)
  }

  /** SetComposeSender が成功した後に呼ぶ。 */
  function onSenderChanged(accountId: string): void {
    conflict.value = undefined
    const body = content.value.bodyHtml
    const sig = signatureOf(accountId)
    const r = replaceSignature(body, { fromSig: inserted, toSig: sig, toAccountId: accountId })
    if (r.status === 'replaced') {
      content.value.bodyHtml = r.html
      inserted = signatureContent(sig)
    } else if (r.status === 'edited') {
      conflict.value = { accountId }
    } else if (inserted === undefined || inserted === '') {
      // 署名ブロックが無いのは、まだ署名を入れていないか、署名なしのアカウントだったから。入れる。
      put(accountId)
    }
    // 署名を入れた後に利用者がブロックを消した場合は、意思を尊重して入れ直さない。
  }

  /** 確認ダイアログの結果。replace=true で編集内容を捨てて差し替える。 */
  function resolve(replace: boolean): void {
    const c = conflict.value
    conflict.value = undefined
    if (c && replace) put(c.accountId)
  }

  return { conflict, init, reset, onSenderChanged, resolve }
}
