// 作成エディタ・送信確認・添付・AI 下書きの文言。
export const compose = {
  paneLabel: 'メール作成',
  titleNew: '新規メール',
  titleReply: '返信',
  titleReplyAll: '全員に返信',
  titleForward: '転送',

  close: '閉じる（下書きを保存）',
  discard: '破棄',
  send: '送信',
  sendAndArchive: '送信してアーカイブ',
  sending: '送信中…',
  saving: '保存中…',
  saved: '下書きを保存しました',
  saveDraft: '下書きを保存',
  senderRequiredHint: '送信元を選択してください',
  discardFailed: '下書きを破棄できませんでした',
  closeFailed: '下書きを保存できなかったため、閉じませんでした。内容は保持しています。',
  saveFailed: '下書きを保存できませんでした',
  openFailed: 'エディタを開けませんでした',
  shortcutHint: 'Ctrl+Enter で送信確認を開きます',

  sender: {
    label: '送信元',
    lockedHint: '返信・転送の送信元は受信アカウントに固定されています',
    unselected: '送信元を選択してください',
    select: '送信元アカウント',
    none: '（未選択）',
  },

  fields: {
    to: '宛先',
    cc: 'Cc',
    bcc: 'Bcc',
    subject: '件名',
    showCc: 'Cc',
    showBcc: 'Bcc',
    hideCc: 'Cc を隠す',
    hideBcc: 'Bcc を隠す',
    addRecipient: (label: string) => `${label} にアドレスを追加`,
    removeRecipient: (email: string) => `${email} を削除`,
    invalidAddress: '形式が正しくないアドレスです',
    suggestions: '宛先の候補',
    subjectPlaceholder: '件名',
  },

  editor: {
    label: '本文',
    bold: '太字',
    link: 'リンク',
    bulletList: '箇条書き',
    linkUrl: 'リンク先 URL',
    linkApply: '設定',
    linkRemove: 'リンクを解除',
    linkCancel: 'キャンセル',
    linkInvalid: 'http、https、mailto のリンクだけ設定できます',
    toolbar: '書式',
  },

  attachments: {
    label: '添付ファイル',
    add: 'ファイルを添付',
    remove: (name: string) => `${name} を削除`,
    total: (size: string) => `合計 ${size}`,
    limit: (size: string) => `上限 ${size}`,
    forwarded: '転送元の添付',
    dropHint: 'ここにファイルをドロップして添付',
    pickFailed: 'ファイルを選択できませんでした',
    tooLarge: (limit: string) => `添付の合計が上限（${limit}）を超えるため追加できませんでした`,
  },

  ai: {
    button: 'AI で下書き',
    generating: 'AI が下書きを作成中…',
    cancel: '生成をキャンセル',
    needKey: 'AI の API キーが未設定です。設定画面で登録してください',
    needLogin: 'ChatGPT にログインしていません。設定画面でログインしてください',
    usingChatGPT: 'Using ChatGPT plan',
    manageUsage: 'Manage usage',
    // 利用枠の超過（subscription_sharing_usage_limit_exceeded）。文言（英語）は OpenAI の UI/UX ガイドラインに合わせ、
    // Manage usage を主なアクションにする。Hitomado に独自のクレジットは無いので、購入の導線は出さない。
    usageLimit: {
      identity: 'ChatGPT',
      title: 'Usage limit reached',
      body: 'Review your ChatGPT plan or app limit in ChatGPT settings.',
      note: 'Your message text has not been changed.',
      dismiss: 'Dismiss',
    },
    openSettings: '設定を開く',
    unavailableHint: '返信のみ利用できます',
    consentTitle: 'AI プロバイダへの送信に同意',
    consentBody: (provider: string, url: string) =>
      `スレッドの本文の一部・差出人・日時と、あなたの表示名・署名が ${provider}（${url}）へ送信されます。同意しますか？`,
    consentAgree: '同意して生成する',
    consentCancel: 'キャンセル',
    replaceTitle: '本文を置き換えますか？',
    replaceBody: '入力済みの本文が AI の下書きに置き換わります。署名と引用は残ります。',
    replaceOk: '置き換える',
    replaceCancel: 'キャンセル',
    failed: 'AI の下書きを作成できませんでした。本文は変更していません。',
  },

  confirm: {
    title: '送信内容の確認',
    description: '送信元と宛先を確認してください。',
    fromLabel: '送信元',
    to: '宛先',
    cc: 'Cc',
    bcc: 'Bcc',
    subject: '件名',
    attachments: '添付',
    attachmentCount: (n: number) => `${n} 件`,
    bodyPreview: '本文',
    emptySubject: '（件名なし）',
    emptyBody: '（本文なし）',
    archiveNote: '送信後にスレッドをアーカイブします',
    warnings: '確認してください',
    ownAccount: '他の自分のアカウントのアドレスが含まれています',
    sendFrom: (email: string) => `${email} から送信`,
    sendFromArchive: (email: string) => `${email} から送信してアーカイブ`,
    sendingNow: '送信中…',
    cancel: 'キャンセル（編集に戻る）',
  },

  signature: {
    conflictTitle: '署名を差し替えますか？',
    conflictBody:
      '署名の部分が編集されています。送信元の署名に差し替えると、編集内容は失われます。',
    conflictReplace: '差し替える',
    conflictKeep: '編集した署名を残す',
  },

  toast: {
    draftAttachmentsDropped: (limit: string) =>
      `下書きの添付が上限（${limit}）を超えるため復元できませんでした。本文と宛先は復元しています`,
    draftAttachmentsFailed: '下書きの添付を復元できませんでした。本文と宛先は復元しています',
    sent: '送信しました',
    sentArchived: '送信してアーカイブしました',
    archiveFailed: '送信しましたが、アーカイブできませんでした',
    draftSaved: '下書きを保存しました',
  },

  errors: {
    SENDER_REQUIRED: '送信元を選択してください。',
    NO_RECIPIENTS: '宛先を 1 件以上入力してください。',
    INVALID_ADDRESS: (emails: string[]) =>
      emails.length > 0
        ? `メールアドレスの形式が正しくありません: ${emails.join('、')}`
        : 'メールアドレスの形式が正しくありません。',
    ATTACHMENT_TOO_LARGE: (limit: string) =>
      `添付の合計が上限（${limit}）を超えています。ファイルを減らしてください。`,
    SENDER_MISMATCH: '送信元がスレッドのアカウントと一致しません。エディタを開き直してください。',
    SENDER_LOCKED: '返信・転送では送信元を変更できません。',
    SESSION_NOT_FOUND: '作成セッションが見つかりません。エディタを開き直してください。',
    CONFIRM_REQUIRED: '送信の確認が必要です。もう一度「送信」を押して確認してください。',
    CONFIRM_EXPIRED: '確認の有効期限が切れました。もう一度「送信」を押して内容を確認してください。',
    CONFIRM_MISMATCH:
      '確認後に内容が変わりました。もう一度「送信」を押して内容を確認してください。',
    SEND_UNCERTAIN:
      '送信済みの可能性があります。送信済みを確認してから、必要な場合だけ再送してください。',
    changedWhilePreparing: '確認の準備中に内容が変更されました。もう一度「送信」を押してください。',
    sendFailed: '送信に失敗しました。内容は保持しています。',
  },
} as const
