// スレッド表示（components/thread）の文言。
export const thread = {
  empty: 'スレッドを選択してください',
  emptyHint: '左の一覧からメールを選ぶと、ここに内容が表示されます。',
  loading: 'スレッドを読み込んでいます',
  loadFailed: 'スレッドを読み込めませんでした。',
  retry: '再試行',
  notFound: 'スレッドが見つかりません',
  noSubject: '（件名なし）',
  labelFailed: 'ラベルを変更できませんでした。',
  labelsLoadFailed: 'ラベルを取得できませんでした。',
  labelsLoading: 'ラベルを読み込み中',
  labelsEmpty: 'ラベルがありません',

  // ヘッダの操作
  archive: 'アーカイブ',
  trash: 'ゴミ箱へ移動',
  label: 'ラベル',
  markUnread: '未読にする',
  markRead: '既読にする',
  receivedAt: '受信アカウント',
  messageCount: (n: number) => `${n} 件のメッセージ`,

  // メッセージカード
  from: '差出人',
  to: '宛先',
  cc: 'Cc',
  toMe: '（宛先なし）',
  expand: 'メッセージを展開',
  collapse: 'メッセージを折りたたむ',
  unreadMark: '未読',
  attachmentCount: (n: number) => `添付 ${n} 件`,
  noBody: '（本文なし）',

  // 本文
  showImages: '画像を表示',
  imagesBlocked: 'リモート画像をブロックしました。',
  bodyFrameTitle: 'メール本文',

  // 添付
  attachments: '添付ファイル',
  saveAttachment: '保存',
  saveAttachmentLabel: (name: string) => `${name} を保存`,
  attachmentSaved: '添付ファイルを保存しました。',
  attachmentFailed: '添付ファイルを保存できませんでした。',

  // 返信
  replyAll: '全員に返信',
  replyMenu: '返信の種類',
  reply: '返信',
  forward: '転送',
  openDraft: '下書きを開く',
  openFailed: 'リンクを開けませんでした。',
} as const
