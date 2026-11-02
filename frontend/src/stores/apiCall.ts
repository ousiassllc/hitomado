// ストア共通のバインディング呼び出し: AppError に統一し、OFFLINE／NEEDS_REAUTH の共通反映と成功時のオンライン復帰を行う。
import { type AppError, call, parseBindingError } from '../lib/api'
import { applyErrorEffects } from '../lib/errorEffects'
import type { useUiStore } from './ui'

export function makeApi(
  ui: ReturnType<typeof useUiStore>,
  markNeedsReauth: (accountId: string) => void,
) {
  return async function api<T>(p: PromiseLike<T>): Promise<T> {
    try {
      const v = await call(p)
      if (!ui.online) ui.setOnline(true)
      return v
    } catch (e) {
      const err: AppError = parseBindingError(e)
      applyErrorEffects(err, { setOnline: ui.setOnline, markNeedsReauth })
      throw err
    }
  }
}
