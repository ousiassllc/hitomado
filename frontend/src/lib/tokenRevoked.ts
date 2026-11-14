// Google がリフレッシュトークンを拒否した（invalid_grant）ことによる要再接続かどうかの判定。
// Go 側（sync の MsgTokenRevoked）が statusMessage に載せる文の先頭と一致させる。
const TOKEN_REVOKED_PREFIX = '再接続が必要です（Google がトークンを無効にしました）'

/** statusMessage が「Google がトークンを無効にした」ことを示すか。原因の案内（OAuth 同意画面の公開ステータスなど）を添える。 */
export function isTokenRevokedMessage(message: string | undefined): message is string {
  return !!message && message.startsWith(TOKEN_REVOKED_PREFIX)
}
