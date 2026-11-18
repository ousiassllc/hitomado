// 設定「通知するメール」（メインだけ／すべて。既定はメインだけ。#395）の保存。値は AppInfo.notifyScope（settingsStore.appInfo）にある。
// 通知を出すかどうかの絞り込みは Go の同期が行う（Windows のトーストも Android のバックグラウンド同期の通知も同じ）。
import { AppService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import { call } from '../lib/api'
import { useSettingsStore } from '../stores/settings'

export type NotifyScope = 'primary' | 'all'

/** 保存値（未設定・未知の値）は「メインだけ」として読む。 */
export function notifyScopeOf(raw: string | undefined): NotifyScope {
  return raw === 'all' ? 'all' : 'primary'
}

/** 保存してから appInfo に反映する。保存に失敗したら appInfo を変えずに投げる。 */
export async function setNotifyScope(scope: NotifyScope): Promise<void> {
  await call(AppService.SetNotifyScope(scope))
  const settings = useSettingsStore()
  if (settings.appInfo) settings.appInfo = { ...settings.appInfo, notifyScope: scope }
}
