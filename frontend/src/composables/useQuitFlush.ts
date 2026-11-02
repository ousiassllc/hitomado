import { AckBeforeQuit } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import { call } from '../lib/api'
import { EVENTS } from '../lib/events'
import { useComposeStore } from '../stores/compose'
import { useWailsEvent } from './useWailsEvents'

/**
 * アプリ終了要求（トレイの「終了」）で未保存の下書きを保存してから Go に完了を伝える（F-12）。
 * 保存に失敗しても必ず AckBeforeQuit を呼ぶ（Go 側の猶予は 3 秒）。
 */
export function useQuitFlush(): void {
  const compose = useComposeStore()
  useWailsEvent<void>(EVENTS.BEFORE_QUIT, () => {
    void compose
      .flush()
      .catch(() => false)
      .then(() => call(AckBeforeQuit()))
      .catch(() => undefined)
  })
}
