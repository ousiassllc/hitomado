// Gmail の権限（スコープ）不足による要再接続かどうかの判定。
// Go 側（apperr の NEEDS_REAUTH／sync の msgScopeInsufficient）が statusMessage に載せる案内文の先頭と一致させる。
const SCOPE_INSUFFICIENT_PREFIX = 'Gmail の権限（スコープ）が足りません'

/** statusMessage が権限（スコープ）不足の案内か。再接続して許可し直すと直る。 */
export function isScopeInsufficientMessage(message: string | undefined): message is string {
  return !!message && message.startsWith(SCOPE_INSUFFICIENT_PREFIX)
}
