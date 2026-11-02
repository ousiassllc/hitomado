import { onScopeDispose } from 'vue'
import { onEvent } from '../lib/events'

/** Wails イベントを購読し、呼び出し元のスコープ（コンポーネント）の破棄時に解除する。 */
export function useWailsEvent<T>(name: string, handler: (data: T) => void): void {
  const off = onEvent<T>(name, handler)
  onScopeDispose(off)
}
