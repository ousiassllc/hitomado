// OAuth クライアント（Client ID／Secret）の拒否による要再接続かどうかの判定。
// Go 側（apperr の OAUTH_CLIENT_REJECTED／sync の msgClientError）が statusMessage に載せる案内文の先頭と一致させる。
const CLIENT_REJECTED_PREFIX = 'OAuth クライアントの設定（Client ID／Secret）'

/** statusMessage が OAuth クライアント設定の案内（資格情報の拒否が原因）か。 */
export function isClientRejectedMessage(message: string | undefined): message is string {
  return !!message && message.startsWith(CLIENT_REJECTED_PREFIX)
}
