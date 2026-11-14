<script setup lang="ts">
import { ArrowLeft } from '@iconoir/vue'
import { TabsContent, TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { computed, watch } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import BaseButton from '../../../ui/BaseButton.vue'
import AISettings from '../AISettings.vue'
import AIFeaturesTab from '../features/AIUseTab.vue'
import AIConnectTab from './AIConnectTab.vue'
import AITypeSafePage from './AITypeSafePage.vue'
import { AI_PROVIDERS, type AIProviderId, isConnected, providerKind } from './aiProviders'

// 設定の AI タブ（#315・#376）。AI の設定は 2 つのタブに分かれる:
//   つなぎ先（connect）     = プロバイダごとの接続（ログイン・API キー・同意）。行の操作で、そのプロバイダの 1 段下のページが開く。
//   機能ごとに使う AI（use） = 機能ごとに、つないである AI のモデルを選ぶ。
// ページを開いている間は、戻る（このボタン・Android の戻る）で「つなぎ先」の一覧へ戻る（開いているページは ui.aiProviderPage。Android の戻るがダイアログを閉じずに戻せるよう、ストアに持つ）。
const ui = useUiStore()
const store = useSettingsStore()
const t = msg.ai

const page = computed(() => ui.aiProviderPage)
const kind = computed(() => (page.value === undefined ? undefined : providerKind(page.value)))

// 開いたときのタブは、つないである AI があれば「機能ごとに使う AI」、何もなければ「つなぎ先」。利用者が選んだら、そのタブ。
const anyConnected = computed(() => AI_PROVIDERS.some((p) => isConnected(p.id, store.aiConfig)))
const tab = computed({
  get: () => ui.aiTab ?? (anyConnected.value ? 'use' : 'connect'),
  set: (v: string) => {
    ui.aiTab = v === 'use' ? 'use' : 'connect'
  },
})

function connect(id: AIProviderId): void {
  ui.openAiProvider(id, true)
}
function open(id: AIProviderId): void {
  ui.openAiProvider(id, false)
}

// 作成画面などの「AI で下書き」の初回の案内から ChatGPT へ進むとき（#104）は、ChatGPT のページから、つなぐ流れを始める。
// すでに ChatGPT につないであるなら、あとは「機能ごとに使う AI」で選ぶだけなので、そのタブを開く。
watch(
  () => ui.aiSetupIntent,
  (intent) => {
    if (intent === undefined) return
    ui.takeAiSetupIntent()
    if (isConnected('chatgpt', store.aiConfig)) ui.aiTab = 'use'
    else ui.openAiProvider('chatgpt', true)
  },
  { immediate: true },
)
</script>

<template>
  <div class="ai-tab">
    <template v-if="page !== undefined && kind !== undefined">
      <div class="head">
        <BaseButton variant="ghost" size="sm" data-testid="ai-provider-back" @click="ui.aiProviderPage = undefined">
          <ArrowLeft class="icon icon-sm" aria-hidden="true" />{{ t.connect.back }}
        </BaseButton>
      </div>
      <AITypeSafePage v-if="kind === 'key'" :key="page" />
      <AISettings v-else :key="page" :provider="page" />
    </template>
    <template v-else>
      <p class="intro">{{ t.intro }}</p>
      <TabsRoot v-model="tab" class="tabs" orientation="horizontal">
        <TabsList class="itab-list" :aria-label="t.tabsLabel">
          <TabsTrigger value="connect" class="itab" data-testid="ai-tab-connect">{{ t.tabs.connect }}</TabsTrigger>
          <TabsTrigger value="use" class="itab" data-testid="ai-tab-use">{{ t.tabs.use }}</TabsTrigger>
        </TabsList>
        <TabsContent value="connect" class="ipanel"><AIConnectTab @connect="connect" @open="open" /></TabsContent>
        <TabsContent value="use" class="ipanel"><AIFeaturesTab /></TabsContent>
      </TabsRoot>
    </template>
  </div>
</template>

<style scoped>
.ai-tab {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.head {
  display: flex;
}

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.tabs {
  display: flex;
  flex-direction: column;
}

/* 設定ダイアログのタブ（SettingsDialog）の 1 段内側。同じ見た目で、文字は 1 段小さい。 */
.itab-list {
  display: flex;
  gap: var(--s-1);
  margin-bottom: var(--s-4);
  border-bottom: 1px solid var(--border);
}

.itab {
  margin-bottom: -1px;
  padding: var(--s-2) var(--s-3);
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--mute);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.itab:hover {
  background: var(--accent-soft);
  color: var(--ink);
}

.itab[data-state='active'] {
  border-bottom-color: var(--accent);
  color: var(--ink);
}

.itab:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}

.ipanel {
  min-height: 0;
}
</style>
