<script setup lang="ts">
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { computed, watch } from 'vue'
import { settings as msg } from '../../lib/messages'
import { useLayoutStore } from '../../stores/layout'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseDialog from '../ui/BaseDialog.vue'
import AccountsSettings from './accounts/AccountsSettings.vue'
import AISettings from './ai/AISettings.vue'
import GeneralSettings from './GeneralSettings.vue'
import LogSettings from './LogSettings.vue'
import McpSettings from './McpSettings.vue'
import OAuthClientsSettings from './oauth/OAuthClientsSettings.vue'
import SignatureSettings from './SignatureSettings.vue'

const ui = useUiStore()
const settingsStore = useSettingsStore()
const platform = usePlatformStore()
const layout = useLayoutStore()

// MCP サーバーを持たない OS（Android）では MCP のタブを出さない。
const tabs = computed(() => [
  { value: 'accounts', label: msg.tabs.accounts, component: AccountsSettings },
  { value: 'oauth', label: msg.tabs.oauthClients, component: OAuthClientsSettings },
  { value: 'signature', label: msg.tabs.signature, component: SignatureSettings },
  { value: 'ai', label: msg.tabs.ai, component: AISettings },
  ...(platform.supportsMcp ? [{ value: 'mcp', label: msg.tabs.mcp, component: McpSettings }] : []),
  { value: 'logs', label: msg.tabs.logs, component: LogSettings },
  { value: 'general', label: msg.tabs.general, component: GeneralSettings },
])

const open = computed({
  get: () => ui.dialogs.settings,
  set: (v: boolean) => {
    if (!v) ui.closeDialog('settings')
  },
})

const tab = computed({
  get: () => ui.settingsTab,
  set: (v: string) => {
    ui.settingsTab = v
  },
})

// 開くたびに最新の AI 設定・アプリ情報を読み込む（開くタブは ui.openSettings が決める）。
watch(open, (isOpen) => {
  if (!isOpen) return
  void settingsStore.load()
})
</script>

<template>
  <BaseDialog v-model:open="open" size="lg" :title="msg.title">
    <TabsRoot v-model="tab" class="tabs" :class="{ full: layout.isStack }" orientation="horizontal">
      <TabsList class="tab-list" :class="{ scroll: layout.isStack }" :aria-label="msg.tabsLabel">
        <TabsTrigger v-for="t in tabs" :key="t.value" :value="t.value" class="tab">
          {{ t.label }}
        </TabsTrigger>
      </TabsList>
      <TabsContent v-for="t in tabs" :key="t.value" :value="t.value" class="panel">
        <component :is="t.component" />
      </TabsContent>
    </TabsRoot>
  </BaseDialog>
</template>

<style scoped>
.tabs {
  display: flex;
  flex-direction: column;
  min-height: 420px;
}

.tab-list {
  display: flex;
  gap: var(--s-1);
  border-bottom: 1px solid var(--border);
  margin-bottom: var(--s-3);
}

.tab {
  padding: var(--s-2) var(--s-3);
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--mute);
  cursor: pointer;
}

.tab:hover {
  background: var(--accent-soft);
}

.tab[data-state="active"] {
  border-bottom-color: var(--accent);
  color: var(--ink);
}

.panel {
  min-height: 0;
}

/* 1 画面ずつ: 全画面のダイアログで、タブは横にスクロールする。 */
.tabs.full {
  min-height: 0;
}

.tab-list.scroll {
  overflow-x: auto;
}

.tab-list.scroll .tab {
  flex: none;
  white-space: nowrap;
}
</style>
