/** 受信箱のタブ。値は InboxQuery.tab（Go 側 store.InboxTab）と同じ文字列。選んだタブは起動をまたいで保存しない。 */
export const INBOX_TABS = ['all', 'primary', 'social', 'promotions', 'new'] as const

export type InboxTab = (typeof INBOX_TABS)[number]
