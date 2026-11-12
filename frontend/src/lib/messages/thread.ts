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
  aiSummarize: 'AI で要約',
  receivedAt: '受信アカウント',
  messageCount: (n: number) => `${n} 件のメッセージ`,

  // メッセージカード
  from: '差出人',
  to: '宛先',
  cc: 'Cc',
  toMe: '（宛先なし）',
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

  // AI パネル（F-17 要約・F-18 質問。#206・#207）
  ai: {
    panelLabel: 'AI パネル',
    summaryHeading: 'AI の要約',
    summarizing: 'AI が要約を作成中…',
    resummarize: '再生成',
    cancel: '中止',
    asking: 'AI が回答を作成中…',
    // 長いスレッドで古いメッセージを外したとき
    truncated: (included: number, total: number) =>
      `長いスレッドのため、新しい ${included} 件（全 ${total} 件）を対象にしました`,
    needKey: 'AI の API キーが未設定です。設定画面で登録してください',
    needLogin: 'ChatGPT にログインしていません。設定画面でログインしてください',
    openSettings: '設定を開く',
    failed: 'AI の処理に失敗しました',
    questionHeading: 'あなたの質問',
    answerHeading: 'AI の回答',
    droppedFromContext: '古いやり取りは文脈から外れています',
    newConversation: '新しい会話',
    question: {
      label: 'このスレッドについて質問する',
      placeholder: '例: 期限はいつ？／相手は何を求めている？',
      note: 'このスレッドを開いている間だけ保持し、保存しません。',
      submit: '質問する',
      remaining: { before: '残り ', after: ' 文字' },
    },
  },
} as const
