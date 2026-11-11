// Inbox Zero（#314）の文言。ボタン・同意・確認の一覧と、設定（AI の TypeSafe AI のページ）の「不要とみなす理由」。
// 判断は TypeSafe AI（Jev）で、候補は確認してからアーカイブする。
export const inboxZero = {
  label: 'Inbox Zero：不要なメールをまとめてアーカイブ',
  blockedNoReason:
    '不要とみなす理由がすべてオフです。設定の AI（TypeSafe AI）でオンにしてください。',
  blockedEmpty: 'この一覧にメールがありません。',
  blockedNoKey: 'TypeSafe AI（Jev）の API キーが未登録です。設定の AI で登録してください。',
  checking: '不要なメールを探しています…',
  none: (n: number) => `不要なメールは見つかりませんでした（${n} 件を確認）。`,
  dialogTitle: 'Inbox Zero：アーカイブする候補',
  dialogDescription: '不要と思われるメールです。残したいものはチェックを外してください。',
  listLabel: 'アーカイブする候補',
  selected: (checked: number, total: number) => `${total} 件中 ${checked} 件を選択中`,
  skipped: (n: number) => `判断できなかった ${n} 件は対象外です。`,
  yes: 'アーカイブする（Yes）',
  no: 'やめる（No）',
  noSubject: '（件名なし）',
  consentTitle: 'TypeSafe AI（Jev）へメールの内容を送ります',
  consentDescription:
    '不要かどうかを判断するため、一覧のメールそれぞれの件名・差出人・本文の先頭約 500 文字を TypeSafe AI（api.typesafe.ai）へ送信します。ボタンを押したときだけ送り、API キーはログにも画面にも出しません。',
  consentAgree: '同意して続ける',
  failed: 'Inbox Zero を実行できませんでした。',
  // 設定: 「不要」とみなす理由。ボタンを押したときは、オンの理由だけで判断する。
  settings: {
    heading: 'Inbox Zero：不要とみなす理由',
    intro:
      'Inbox Zero のボタンを押すと、オンの理由に当てはまるメールを候補として一覧にします。アーカイブするかどうかは、一覧で確認してから決めます。',
    groupLabel: '不要とみなす理由',
    reasons: {
      ad: { label: '広告・宣伝', hint: 'TypeSafe AI が、確信度が高いと判断したものだけ。' },
      notification: {
        label: '通知だけのメール',
        hint: 'TypeSafe AI が、確信度が高いと判断したものだけ。',
      },
      newsletter: {
        label: '未読のニュースレター',
        hint: '未読で、TypeSafe AI が確信度が高いと判断したものだけ。',
      },
      replied: {
        label: '返信済みのメール',
        hint: 'スレッドに自分の送信があるもの（TypeSafe AI には聞きません）。',
      },
    } as Record<string, { label: string; hint: string }>,
    saveFailed: '設定を保存できませんでした。',
  },
}
