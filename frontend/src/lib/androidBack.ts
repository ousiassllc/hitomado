/**
 * Android の戻るボタン（ジェスチャー）の受け口。MainActivity.java が `window.hitomadoHandleBack()` を呼び、
 * true（戻る先があり一つ前の画面へ戻した）ならアプリを閉じず、それ以外はアプリをバックグラウンドへ送る。
 * 名前は MainActivity.java の handleBack と一致させること。
 */
export const BACK_HANDLER_NAME = 'hitomadoHandleBack'

type BackWindow = Window & { [BACK_HANDLER_NAME]?: () => boolean }

/** 受け口を登録する。戻り値で解除する。 */
export function installBackHandler(handler: () => boolean): () => void {
  const w = window as BackWindow
  w[BACK_HANDLER_NAME] = handler
  return () => {
    if (w[BACK_HANDLER_NAME] === handler) delete w[BACK_HANDLER_NAME]
  }
}
