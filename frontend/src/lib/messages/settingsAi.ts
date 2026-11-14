import { settingsChatgpt } from './settingsChatgpt'

// 設定画面の AI の文言（settings.ai として settings.ts から参照する）。
// AI の設定は「AI のつなぎ先」（connect）と「機能ごとに使う AI」（use）の 2 つのタブに分かれる（#376）。表記は「接続済み」「未接続」「つなぎ直しが必要」。
export const settingsAi = {
  heading: 'AI',
  intro:
    'AI は、各機能を使うときだけ、その機能の内容を送信先へ送信します。自動で生成・送信することはありません。',
  tabsLabel: 'AI の設定の分類',
  tabs: { connect: 'AI のつなぎ先', use: '機能ごとに使う AI' },
  destinationHeading: 'データの送信先',
  destination: (provider: string, url: string) => `${provider}（${url}）`,
  destinationDefaultNote: '既定の送信先です。',
  destinationCustomNote: 'Base URL で指定した送信先です。',
  destinationChatGPTNote:
    'スレッドの内容は OpenAI の API（Responses）へ送信されます。ログインだけは auth.openai.com の公式画面で行います。',
  destinationPending: (dest: string) =>
    `入力中の送信先（${dest}）は未保存です。Base URL を保存するまで、上の送信先が使われます。`,
  providerOptions: {
    anthropic: 'Anthropic',
    openai: 'OpenAI 互換',
    chatgpt: 'ChatGPT プラン（Continue with ChatGPT）',
    typesafe: 'TypeSafe AI（Jev）',
  } as Record<string, string>,
  providerList: {
    open: (provider: string) => `${provider} の設定を開く`,
    names: {
      chatgpt: 'ChatGPT',
      anthropic: 'Anthropic',
      openai: 'OpenAI 互換',
      typesafe: 'TypeSafe AI（Jev）',
    } as Record<string, string>,
  },
  // 「AI のつなぎ先」タブ。プロバイダごとの状態と、つなぐ操作だけ。
  connect: {
    heading: 'AI のつなぎ先',
    hint: 'ログインや API キーでつなぎます。モデルは「機能ごとに使う AI」で選びます。',
    listLabel: 'AI のつなぎ先の一覧',
    connected: '接続済み',
    disconnected: '未接続',
    reconnect: 'つなぎ直しが必要',
    login: 'ログイン',
    addKey: 'API キーを追加',
    consent: '同意して接続',
    manage: '設定',
    disconnect: '接続を外す',
    connectFor: (provider: string, action: string) => `${provider}：${action}`,
    disconnectFor: (provider: string) => `${provider} の接続を外す`,
    disconnectTitle: (provider: string) => `${provider} の接続を外しますか？`,
    disconnectDetail:
      '保存済みの API キーが資格情報ストアから削除されます。「機能ごとに使う AI」で選んでいる場合、選択は残り、つなぎ直すまでその機能は使えません。',
    disconnectDetailChatGPT:
      'ログイン中の ChatGPT アカウントをすべてログアウトします。「機能ごとに使う AI」で選んでいる場合、選択は残り、ログインし直すまでその機能は使えません。',
    disconnectedToast: (provider: string) => `${provider} の接続を外しました。`,
    consentNote: '送信への同意がまだです。',
    typesafeNote: 'Inbox Zero 専用',
    back: '「AI のつなぎ先」に戻る',
    pageIntro:
      'ここでは、つなぐ操作（ログイン・API キー・送信への同意）だけを行います。使うモデルは「機能ごとに使う AI」で選びます。',
    baseUrlSave: 'Base URL を保存',
    baseUrlSaved: 'Base URL を保存しました。',
  },
  // 「機能ごとに使う AI」タブ。機能ごとに、つないである AI のモデルを選ぶ。
  use: {
    heading: '機能ごとに使う AI',
    hint: 'つないである AI のモデルだけが一覧に出ます。つなぐ操作は「AI のつなぎ先」にあります。',
    listLabel: '機能ごとに使う AI の一覧',
    empty: 'まず AI をつなぐと、ここで機能ごとに選べます。',
    connect: 'AI をつなぐ',
    noneAvailable: '使える AI がありません',
    reconnect: 'つなぎ直しが必要',
    relogin: 'ログインし直す',
    fixConnection: 'つなぎ先を直す',
    placeholder: 'AI とモデルを選択',
    selectLabel: (feature: string) => `${feature}に使う AI とモデル`,
    loading: 'モデルの一覧を取得しています…',
    manualLabel: (provider: string) => `${provider} のモデル名`,
    manualSave: 'モデルを保存',
    fixed: (provider: string) => `${provider}（固定）`,
    soon: '準備中',
    features: { draft: 'AI で下書き', inboxZero: 'Inbox Zero', agent: 'エージェント' },
  },
  typesafe: {
    note: 'TypeSafe AI（Jev）は文章を書かないモデルです。「AI で下書き」には選べず、Inbox Zero の「機能ごとに使う AI」にだけ選べます（不要なメールの判断に使います）。',
  },
  modelPlaceholder: '例: claude-sonnet-4-5',
  modelRequired: 'モデルを入力してください。',
  // モデルのエフォート（#111）。先頭は常に「モデルの既定」（何も送らない）。
  effort: 'エフォート',
  effortDefault: 'モデルの既定',
  effortLabels: {
    low: 'low（控えめ）',
    medium: 'medium（標準）',
    high: 'high（深く）',
    xhigh: 'xhigh（より深く）',
    max: 'max（最大）',
  } as Record<string, string>,
  modelNotListed: (id: string) => `${id}（一覧にありません）`,
  modelsManualHint: '一覧から選べないときは、モデル名を直接入力できます。',
  baseUrl: 'Base URL（OpenAI 互換のみ）',
  baseUrlPlaceholder: '空欄で OpenAI の既定（https://api.openai.com）',
  baseUrlHint: 'OpenAI 互換のサービスを使うときだけ指定します。',
  apiKeyHeading: 'API キー',
  apiKeyFor: (provider: string) => `${provider} の API キー`,
  apiKeyPlaceholder: 'API キーを入力',
  apiKeyStatusSet: '保存済み',
  apiKeyStatusUnset: '未設定',
  apiKeySave: 'キーを保存',
  apiKeyClear: 'キーを削除',
  apiKeyClearTitle: (provider: string) => `${provider} の API キーを削除しますか？`,
  apiKeyClearDetail:
    '保存済みの API キーが資格情報ストアから削除されます。登録し直すまで、この AI は使えなくなります。',
  apiKeyClearConfirm: '削除する',
  apiKeySaved: 'API キーを保存しました。',
  apiKeyCleared: 'API キーを削除しました。',
  apiKeyRequired: 'API キーを入力してください。',
  apiKeyStored: 'API キーは資格情報ストアに保存され、画面には再表示されません。',
  chatgpt: settingsChatgpt,
  // つなぐときの同意（F-08）。同意の画面の文言は compose.ai の consentTitle／consentBody と共通。
  consentDialogAgree: '同意する',
  consentNeeded: (provider: string) =>
    `${provider} への送信は未同意です。つなぐには、送信先への同意が必要です。`,
  consentAgreeNow: (dest: string) => `${dest} への送信に同意する`,
  consentHeading: '送信への同意',
  consentGiven: (provider: string) => `${provider} への送信に同意済みです。`,
} as const
