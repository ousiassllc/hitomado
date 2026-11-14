// Inbox Zero で「不要」とみなす理由（#314）。表示順どおり。キーは Go の dto.InboxZeroReasons の json と同じ。
// ad・notification・newsletter は TypeSafe AI（Jev）が判断し、replied はスレッドに自分の送信があるかで Go が決める。
export const INBOX_ZERO_REASONS = ['ad', 'notification', 'newsletter', 'replied'] as const

export type InboxZeroReason = (typeof INBOX_ZERO_REASONS)[number]
