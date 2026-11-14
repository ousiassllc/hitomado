// エージェントモード（パネル・設定・同意）の文言（#372）。
export const agent = {
  // サイドバー下の入口の一言（見出しは nav.agent.label）。
  entry: {
    off: 'オフ',
    on: 'オン',
    pending: (n: number) => `確認待ち ${n}件`,
  },
  // 下タブのエージェント。件数があるときは、バッジの件数を読み上げに含める。
  tab: {
    pending: (n: number) => `エージェント（確認待ち ${n}件）`,
    badge: (n: number) => `確認待ち ${n}件`,
  },
  panel: {
    off: {
      title: 'エージェントはオフです',
      body: 'オンにすると、新着メールの内容が設定済みの AI に送られ、返信が要るメールには下書きまで自動で作ります。',
      note: '送信は必ず自分で押します。エージェントが送信したり、自動で返信したりすることはありません。',
      openSettings: '設定でオンにする',
    },
    empty: {
      title: '確認するものはありません',
      body: '返信が要りそうな新着メールがあると、下書きや質問をここに出します。',
    },
    sections: {
      asking: '質問',
      ready: '確認待ち',
      later: 'あとで',
      skipped: '返信不要と判断',
      failed: '失敗',
    },
    noSubject: '（件名なし）',
    from: (name: string) => `差出人: ${name}`,
    to: (names: string) => `宛先: ${names}`,
    receivedBy: '受信',
    questionsHint: '決められないことがあります。答えを書くと、それをもとに下書きを作ります。',
    answerPlaceholder: '答えを入力',
    answerSubmit: '答えて下書きを作る',
    defer: 'あとで',
    openDraft: '開いて確認',
    reconsider: 'やっぱり返信する',
    retry: 'やり直す',
    dismiss: '閉じる',
    noDraft: '開ける下書きがありません。',
    count: (label: string, n: number) => `${label}（${n}件）`,
  },
  settings: {
    tab: 'エージェント',
    heading: 'エージェントモード',
    switchLabel: 'エージェントモード',
    description:
      'オンにすると、ボタンを押さなくても、新着メールの返信が要るかを AI が判断し、要るものには下書きを作ります。送信は必ず自分で押します。',
    scope:
      '対象は受信箱（メイン）の新着だけです。プロモーション・ソーシャル・自分が送ったメールは除きます。',
    aiNotReady: 'AI の設定が使えない状態です。AI の設定を確認するまで、エージェントは動きません。',
    openAiSettings: 'AI の設定を開く',
    needAiTitle: '先に AI の設定が必要です',
    needAiBody:
      'エージェントは設定済みの AI で動きます。AI のプロバイダとモデルを設定してから、あらためてオンにしてください。',
    needAiOpen: 'AI の設定へ',
    needAiCancel: 'あとで',
    mascotSwitchLabel: 'マスコットを画面の端に出す',
    mascotHeading: 'マスコット',
    mascotDescription:
      '確認待ちがあるとき、画面の端のマスコットでお知らせします。押すとエージェントのパネルを開きます。',
    mascotNeedsAgent: 'マスコットを使うには、先にエージェントモードをオンにしてください。',
  },
  consent: {
    title: 'エージェントモードをオンにする',
    body: (provider: string, url: string) =>
      `新着メールの件名・差出人・本文が、設定済みの AI プロバイダ（${provider}・${url}）へ自動で送られます。`,
    draft:
      '返信が要るメールには、下書きまで自動で作ります。送信は必ず自分で押します。エージェントは送信しません。',
    off: 'いつでもオフにできます。',
    agree: '同意してオンにする',
    cancel: 'キャンセル',
  },
} as const
