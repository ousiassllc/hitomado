<script setup lang="ts">
import './styles/tokens.css'
import './styles/reset.css'
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
  <AppShell />
</template>
