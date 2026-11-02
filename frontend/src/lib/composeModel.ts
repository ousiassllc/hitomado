// composeStore が使う型と、バインディング値の正規化（null → 空配列）。
import type {
  ComposeContent,
  ComposeSession,
  OpenComposeRequest,
  SendPreview,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import type { Address } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { list } from './api'

/** エディタが編集する内容。ComposeContent と同形で、配列は常に非 null。 */
export interface EditableContent {
  to: Address[]
  cc: Address[]
  bcc: Address[]
  subject: string
  bodyHtml: string
}

/** 返信・転送の対象スレッド。 */
export interface ThreadTarget {
  accountId: string
  threadId: string
}

export function emptyContent(): EditableContent {
  return { to: [], cc: [], bcc: [], subject: '', bodyHtml: '' }
}

export function normalizeContent(c: ComposeContent): EditableContent {
  return {
    to: list(c.to),
    cc: list(c.cc),
    bcc: list(c.bcc),
    subject: c.subject,
    bodyHtml: c.bodyHtml,
  }
}

export function normalizePreview(p: SendPreview): SendPreview {
  return { ...p, to: list(p.to), cc: list(p.cc), bcc: list(p.bcc), warnings: list(p.warnings) }
}

export function normalizeSession(s: ComposeSession): ComposeSession {
  return { ...s, attachments: list(s.attachments) }
}

/** バインディングへ渡す内容（リアクティブ参照を切り離したコピー）。 */
export function snapshotOf(c: EditableContent): ComposeContent {
  return {
    to: c.to.map((a) => ({ ...a })),
    cc: c.cc.map((a) => ({ ...a })),
    bcc: c.bcc.map((a) => ({ ...a })),
    subject: c.subject,
    bodyHtml: c.bodyHtml,
  }
}

/** 新規作成の送信元の初期値（F-11）: 受信箱のアカウントフィルタが単一選択ならそのアカウント、それ以外は未選択。 */
export function withDefaultSender(
  req: OpenComposeRequest,
  filterAccountIds: string[],
): OpenComposeRequest {
  if (req.mode !== 'new' || req.accountId || filterAccountIds.length !== 1) return req
  return { ...req, accountId: filterAccountIds[0] }
}
