// ナビ（サイドバー・スマホの下タブ・その他）の文言。見る場所とアカウントの絞り込みは別の軸（#370）。
export const nav = {
  places: {
    label: '見る場所',
    inbox: '受信箱',
    drafts: '下書き',
    sent: '送信済み',
    archive: 'アーカイブ',
  },
  accounts: {
    heading: 'アカウント',
    all: '全アカウント',
  },
  agent: {
    label: 'エージェント',
    panelTitle: 'エージェント',
    close: 'エージェントを閉じる',
  },
  tabs: {
    label: 'メインメニュー',
    inbox: '受信箱',
    drafts: '下書き',
    agent: 'エージェント',
    more: 'その他',
  },
} as const
