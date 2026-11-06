<script setup lang="ts">
// フォント（Sora・Noto Sans JP・JetBrains Mono。すべて SIL OFL）。CSP の font-src 'self' で読めるようアプリに同梱する。
import '@fontsource-variable/sora/wght.css'
import '@fontsource-variable/noto-sans-jp/wght.css'
import '@fontsource-variable/jetbrains-mono/wght.css'
import './styles/tokens.css'
import './styles/reset.css'
import './styles/scrollbar.css'
import './styles/touch.css'
import { IconoirProvider } from '@iconoir/vue'
import { onMounted } from 'vue'
import AppShell from './components/shell/AppShell.vue'
import { useQuitFlush } from './composables/useQuitFlush'
import { useTheme } from './composables/useTheme'
import { useAccountsStore } from './stores/accounts'
import { useInboxStore } from './stores/inbox'
import { usePlatformStore } from './stores/platform'
import { useSettingsStore } from './stores/settings'
import { useThreadStore } from './stores/thread'

useTheme()
useQuitFlush()

const accounts = useAccountsStore()
const inbox = useInboxStore()
const thread = useThreadStore()
const settings = useSettingsStore()
const platform = usePlatformStore()

// 各ストアの start() はイベント購読＋初期ロード（冪等）。1 つの失敗で他を止めない。
onMounted(() => {
  thread.start()
  void Promise.allSettled([platform.start(), accounts.start(), inbox.start(), settings.start()])
})
</script>

<template>
  <!-- 機能の読み込み（Android かどうか）が終わってから描く。Android で一瞬 3 ペインを出さない。 -->
  <!-- アイコン（Iconoir）は currentColor・ストローク 1.5。大きさは CSS の --icon-sm/md/lg（styles/reset.css の .icon） -->
  <IconoirProvider :icon-props="{ color: 'currentColor', strokeWidth: 1.5 }">
    <AppShell v-if="platform.settled" />
  </IconoirProvider>
</template>
