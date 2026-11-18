<script setup lang="ts">
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { computed, ref, watch } from 'vue'
import { settings as msg } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import { useLayoutStore } from '../../stores/layout'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseDialog from '../ui/BaseDialog.vue'
import { revealInline } from '../ui/revealInline'
import AccountsSettings from './accounts/AccountsSettings.vue'
import AIProviders from './ai/providers/AIProviders.vue'
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
  { value: 'ai', label: msg.tabs.ai, component: AIProviders },
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

// スクロールバーを 1 秒出す合図。AI のタブは中にも内側のタブ（つなぎ先／機能ごと）と、開いたプロバイダのページがあり、それを切り替えると本文の長さが変わるので、タブと同じく合図を変える。
const scrollKey = computed(() =>
  tab.value === 'ai' ? `ai:${ui.aiTab ?? ''}:${ui.aiProviderPage ?? ''}` : tab.value,
)

// 1 画面ずつ（390 幅など）はタブ行が横にスクロールするので、開いた直後・タブの切り替え・向きの変更のとき、選んでいるタブが見える位置に来るようにする（#383）。
// 横にスクロールしない幅（デスクトップ）では何もしない。フォントの読み込みでタブの幅が変わると位置がずれるので、タブの大きさが変わったときも合わせ直す。
const tabList = ref<{ $el: HTMLElement } | null>(null)
watch(
  [() => tabList.value?.$el, open, tab, () => layout.isStack],
  ([list, isOpen, , stack], _prev, onCleanup) => {
    if (!list || !isOpen || !stack) return
    const reveal = (): void => {
      const active = list.querySelector<HTMLElement>('[data-state="active"]')
      if (active) revealInline(list, active)
    }
    reveal()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(reveal)
    for (const el of list.children) observer.observe(el)
    onCleanup(() => observer.disconnect())
  },
  { flush: 'post' },
)

// 開くたびに最新の AI 設定・アプリ情報を読み込む（開くタブは ui.openSettings が決める）。
watch(open, (isOpen) => {
  if (!isOpen) return
  void settingsStore.load()
})
</script>

<template>
  <BaseDialog v-model:open="open" size="lg" :title="msg.title" :scroll-key="scrollKey" scroll-fade>
    <TabsRoot v-model="tab" class="tabs" :class="{ full: layout.isStack }" orientation="horizontal">
      <TabsList ref="tabList" v-overlay-scroll class="tab-list" :class="{ scroll: layout.isStack }" :aria-label="msg.tabsLabel">
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
  display: flex;
  align-items: center;
  /* ガイドのタブ: 高さ 40（--control-h。下線 2px を含む）・左右 padding --s-3 */
  height: var(--control-h);
  padding: 0 var(--s-3);
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--mute);
  cursor: pointer;
}

.tab:hover {
  background: var(--accent-soft);
  /* --accent-soft 地の上では --mute が AA 4.5:1 に届かないので --ink にする（選択タブは下線 --accent で区別が残る）。 */
  color: var(--ink);
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
  /* タブの offsetLeft の基準（選んでいるタブを見える位置へ動かす revealInline）。 */
  position: relative;
}

.tab-list.scroll .tab {
  flex: none;
  white-space: nowrap;
}

/* 横スクロール（overflow-x: auto）の中なので、外側のリングは上下で切れる。内側に描く（docs §5.3 逸脱）。 */
.tab-list.scroll .tab:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}
</style>
