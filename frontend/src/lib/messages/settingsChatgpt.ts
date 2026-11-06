// 設定画面の AI のうち、ChatGPT プラン（Continue with ChatGPT）の文言。settings.ai.chatgpt として settings.ts から参照する。
export const settingsChatgpt = {
  heading: 'ChatGPT プラン',
  intro:
    'ChatGPT Plus／Pro のプランの範囲で、API キーなしで下書きを作ります。ログインは OpenAI の公式画面で行い、Hitomado はパスワードも Cookie も受け取りません。',
  notOfficial:
    'Hitomado は OpenAI の公式アプリではありません。許可は ChatGPT の設定でいつでも取り消せます。',
  login: 'Continue with ChatGPT',
  loginWaiting: 'ブラウザで ChatGPT のログインを完了してください…',
  loginCancel: 'ログインを中止',
  addAccount: 'アカウントを追加',
  accountsHeading: 'ChatGPT アカウント',
  accountsLabel: '下書きに使う ChatGPT アカウント',
  accountHint:
    'アカウントを追加しても、いま使っているアカウントは、追加のログインが終わるまで変わりません。同じメールアドレスでも別の登録として扱います。',
  noEmail: '（メールアドレスなし）',
  accountDuplicate: (label: string, id: string) => `${label}（登録 …${id.slice(-4)}）`,
  accountActive: '使用中',
  accountSignedOut: 'ログアウト済み',
  select: (label: string) => `${label} を使う`,
  relogin: '再ログイン',
  logout: 'ログアウト',
  logoutAccount: (label: string) => `${label} をログアウト`,
  logoutTitle: (label: string) => `${label} をログアウトしますか？`,
  logoutDetail:
    'OpenAI 側のセッションを失効させ、このアプリに保存されているトークンを削除します。もう一度使うには、ブラウザでログインし直してください。',
  logoutConfirm: 'ログアウトする',
  reloginAccount: (label: string) => `${label} に再ログイン`,
  remove: '削除',
  removeAccount: (label: string) => `${label} の登録を削除`,
  removed: 'ChatGPT アカウントの登録を削除しました。',
  removeTitle: (label: string) => `${label} の登録を削除しますか？`,
  removeDetail:
    'このアプリに保存されているこの ChatGPT アカウントの登録（ログアウト済みのもの）を忘れます。使うには、もう一度ログインして登録し直してください。',
  removeConfirm: '削除する',
  switched: (label: string) => `${label} に切り替えました。`,
  // ログアウト済みのアカウントを選んだとき（一覧が古いなど）。ボタン名は一覧の「再ログイン」と同じ。
  selectSignedOut:
    'このアカウントはログアウト済みです。「再ログイン」からもう一度ログインしてください。',
  statusSignedIn: (email: string) => (email ? `ログイン済み（${email}）` : 'ログイン済み'),
  statusSignedOut: '未ログインです。',
  statusChoose: '使うアカウントを選んでください。',
  manageUsage: 'Manage usage',
  usageUrl: 'https://chatgpt.com/settings/usage',
  // 初めて ChatGPT プランの利用が有効になったときの確認。文言（英語）は OpenAI の UI/UX ガイドラインに合わせる。
  planNotice: {
    title: "You're using your ChatGPT plan",
    body: 'Eligible AI requests in Hitomado will use your ChatGPT plan. You can manage usage in ChatGPT settings.',
    account: (email: string) => `ChatGPT account: ${email}`,
    note: 'Hitomado の「AI で下書き」は、あなたの ChatGPT プランの範囲で使われます。使用状況は ChatGPT の設定で確認・管理できます。',
    gotIt: 'Got it',
    ackFailed: '確認の記録に失敗しました。次回もう一度表示されます。',
  },
  signedIn: 'ChatGPT にログインしました。',
  signedOut: 'ChatGPT からログアウトしました。',
  revokeUnconfirmed:
    'ログアウトしました（OpenAI 側の連携の解除は確認できませんでした）。ChatGPT の設定の連携アプリから解除できます。',
} as const
