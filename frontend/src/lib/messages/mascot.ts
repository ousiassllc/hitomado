// 画面の端のマスコット（別ウィンドウ）の文言（#372）。
export const mascot = {
  /** 押すと、メインウィンドウのエージェントのパネルを開く。件数があれば読み上げに含める。 */
  openLabel: (pending: number) =>
    pending > 0 ? `エージェントを開く・確認待ち ${pending} 件` : 'エージェントを開く',
  /** 件数バッジの読み上げ（バッジ自体は見た目だけ。名前はボタンのものを使う）。 */
  pendingLabel: (pending: number) => `確認待ち ${pending} 件`,
  /** ✕ ボタン。画面の端から消す（設定でもオフにできる）。 */
  hideLabel: 'マスコットを消す',
} as const
