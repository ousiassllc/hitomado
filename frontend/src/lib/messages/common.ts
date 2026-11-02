export const common = {
  appName: 'Hitomado',
  cancel: 'キャンセル',
  close: '閉じる',
  save: '保存',
  delete: '削除',
  retry: '再試行',
  offlineBanner: 'オフラインです。取得済みの内容を表示しています。',
  needsReauthBanner: (name: string) => `${name} の接続が切れました。再接続してください。`,
  reconnect: '再接続',
} as const
