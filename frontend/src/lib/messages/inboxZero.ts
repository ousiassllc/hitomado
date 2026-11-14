// Inbox Zero（#314・#377）の文言。ボタン・同意・確認の一覧と、設定（AI の「機能ごとに使う AI」タブ）の「不要とみなす理由」。
// 広告・通知・ニュースレターの判断は「機能ごとに使う AI」で Inbox Zero に選んだ AI、返信済み・カレンダーは端末内で判断し、
// 候補は確認してからアーカイブする。
export const inboxZero = {
  label: 'Inbox Zero：不要なメールをまとめてアーカイブ',
  blockedNoReason:
    '不要とみなす理由がすべてオフです。設定の AI の「機能ごとに使う AI」でオンにしてください。',
  blockedEmpty: 'この一覧にメールがありません。',
  blockedNoAI:
    'Inbox Zero で使う AI が選ばれていません。設定の AI の「機能ごとに使う AI」で選んでください。',
  blockedNotConnected: (provider: string) =>
    `Inbox Zero に選んだ ${provider} がつながっていません。設定の AI の「AI のつなぎ先」で確認してください。`,
  checking: '不要なメールを探しています…',
  checkingTitle: 'Inbox Zero：判断中',
  abort: 'やめる',
  none: (n: number) => `不要なメールは見つかりませんでした（${n} 件を確認）。`,
  dialogTitle: 'Inbox Zero：アーカイブする候補',
  dialogDescription: '不要と思われるメールです。残したいものはチェックを外してください。',
  listLabel: 'アーカイブする候補',
  selected: (checked: number, total: number) => `${total} 件中 ${checked} 件を選択中`,
  skipped: (n: number) => `判断できなかった ${n} 件は対象外です。`,
  yes: 'アーカイブする（Yes）',
  no: 'やめる（No）',
  noSubject: '（件名なし）',
  consentTitle: (provider: string) => `${provider} へメールの内容を送ります`,
  consentDescription: (provider: string, url: string) =>
    `不要かどうかを判断するため、一覧のメールそれぞれの件名・差出人・本文の先頭約 500 文字を ${provider}（${url}）へ送信します。ボタンを押したときだけ送り、API キー・ログイン情報はログにも画面にも出しません。返信済みとカレンダーの判断は端末内で行い、送信しません。`,
  consentAgree: '同意して続ける',
  failed: 'Inbox Zero を実行できませんでした。',
  // 設定: 「不要」とみなす理由。ボタンを押したときは、オンの理由だけで判断する。
  settings: {
    heading: 'Inbox Zero：不要とみなす理由',
    intro:
      'Inbox Zero のボタンを押すと、オンの理由に当てはまるメールを候補として一覧にします。アーカイブするかどうかは、一覧で確認してから決めます。',
    groupLabel: '不要とみなす理由',
    reasons: {
      ad: { label: '広告・宣伝', hint: '選んだ AI が、確信度が高いと判断したものだけ。' },
      notification: {
        label: '通知だけのメール',
        hint: '選んだ AI が、確信度が高いと判断したものだけ。',
      },
      newsletter: {
        label: '未読のニュースレター',
        hint: '未読で、選んだ AI が確信度が高いと判断したものだけ。',
      },
      replied: {
        label: '返信済みのメール',
        hint: 'スレッドの最後のメール（下書きを除く）が自分の送信のもの（AI には聞きません）。',
      },
      calendar: {
        label: 'カレンダーの招待・通知',
        hint: '最後のメールがカレンダーの招待・通知のもの（AI には聞かず、端末内で判断します）。',
      },
    } as Record<string, { label: string; hint: string }>,
    saveFailed: '設定を保存できませんでした。',
  },
}
