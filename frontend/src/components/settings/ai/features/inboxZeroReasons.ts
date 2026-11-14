// Inbox Zero で「不要」とみなす理由（#314・#377）。表示順どおり。キーは Go の dto.InboxZeroReasons の json と同じ。
// ad・notification・newsletter は「機能ごとに使う AI」で Inbox Zero に選んだ AI が判断し、replied（スレッドの最後が自分の送信）と
// calendar（カレンダーの招待・通知）は AI を使わず Go が端末内で決める。
export const INBOX_ZERO_REASONS = [
  'ad',
  'notification',
  'newsletter',
  'replied',
  'calendar',
] as const

export type InboxZeroReason = (typeof INBOX_ZERO_REASONS)[number]

/** AI に聞く理由（1 つでもオンなら、Inbox Zero に選んだ AI が使える必要がある）。 */
export const INBOX_ZERO_AI_REASONS: readonly InboxZeroReason[] = [
  'ad',
  'notification',
  'newsletter',
]
