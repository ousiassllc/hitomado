// 設定「送信前に確認画面を出す」（既定オン。#313）の保存。値は AppInfo.sendConfirmEnabled（settingsStore.appInfo）にある。
// 送信フロー（useComposeSend）はこの値がオフで警告が無いときだけ確認画面を省く。
import { AppService } from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import { call } from '../lib/api'
import { useSettingsStore } from '../stores/settings'

/** 保存してから appInfo に反映する。保存に失敗したら appInfo を変えずに投げる。 */
export async function setSendConfirmEnabled(enabled: boolean): Promise<void> {
  await call(AppService.SetSendConfirmEnabled(enabled))
  const settings = useSettingsStore()
  if (settings.appInfo) settings.appInfo = { ...settings.appInfo, sendConfirmEnabled: enabled }
}
