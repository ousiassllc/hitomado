import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as NotifyService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/notifycore/notifyservice'
import { call, parseBindingError } from '../lib/api'
import { settings as msg } from '../lib/messages'
import { useUiStore } from './ui'

/** 通知の許可状態。unsupported は許可を管理しない OS（Windows）で、設定画面に項目を出さない。 */
export type NotificationPermission = 'granted' | 'denied' | 'unsupported'

/**
 * Android の通知の許可（POST_NOTIFICATIONS と OS の通知設定）の状態と、許可の要求・OS の設定を開く操作。
 * 画面に出すかどうかは permission !== 'unsupported' で決める（OS 名は比べない）。
 */
export const useNotificationsStore = defineStore('notifications', () => {
  const ui = useUiStore()
  const permission = ref<NotificationPermission>('unsupported')

  /** 現在の許可状態を読み直す。失敗したら項目を出さない（unsupported）。 */
  async function refresh(): Promise<void> {
    try {
      const p = await call(NotifyService.GetNotificationPermission())
      permission.value = p === 'granted' || p === 'denied' ? p : 'unsupported'
    } catch {
      permission.value = 'unsupported'
    }
  }

  /**
   * 許可のダイアログを出す。結果は待たず、少し遅れて状態を読み直す（呼び出し側が復帰イベントで refresh する）。
   * 永続的に拒否済みの場合はダイアログが出ないため、案内に従って OS の設定を開く。
   */
  async function request(): Promise<void> {
    try {
      await call(NotifyService.RequestNotificationPermission())
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message)
    }
    await refresh()
  }

  async function openSettings(): Promise<void> {
    try {
      await call(NotifyService.OpenNotificationSettings())
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message || msg.notifications.openFailed)
    }
  }

  return { permission, refresh, request, openSettings }
})
