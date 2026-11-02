import { onBeforeUnmount, onMounted } from 'vue'
import { useUiStore } from '../stores/ui'

/** OS の配色設定（prefers-color-scheme）に追従し、uiStore.theme と color-scheme を更新する。配色自体は tokens.css が切り替える。 */
export function useTheme(): void {
  const ui = useUiStore()
  const query = window.matchMedia('(prefers-color-scheme: dark)')

  function apply(): void {
    ui.theme = query.matches ? 'dark' : 'light'
    document.documentElement.style.colorScheme = ui.theme
  }

  apply()
  onMounted(() => query.addEventListener('change', apply))
  onBeforeUnmount(() => query.removeEventListener('change', apply))
}
