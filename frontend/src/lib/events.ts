// Go → フロントのイベント（バインディング仕様 §8）の購読ラッパー。
import { Events } from '@wailsio/runtime'

export const EVENTS = {
  SYNC_UPDATED: 'sync:updated',
  ACCOUNT_STATUS: 'account:status',
  BEFORE_QUIT: 'app:before-quit',
  NOTIFY_OPEN_THREAD: 'notify:open-thread',
  UPDATE_AVAILABLE: 'update:available',
} as const

export type AccountStatus = 'active' | 'syncing' | 'needs_reauth' | 'error'

export interface SyncUpdatedPayload {
  accountId: string
  changedThreadIds: string[]
}

export interface AccountStatusPayload {
  accountId: string
  status: AccountStatus
  message?: string
}

export interface NotifyOpenThreadPayload {
  accountId: string
  threadId: string
}

export interface UpdateAvailablePayload {
  version: string
  url: string
}

/**
 * イベントを購読し、解除関数を返す。ハンドラには `WailsEvent.data`（ペイロード）を渡す。
 */
export function onEvent<T>(name: string, handler: (data: T) => void): () => void {
  return Events.On(name, (ev) => handler(ev.data as T))
}
