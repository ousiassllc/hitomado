<script setup lang="ts">
import { ArrowLeft } from '@iconoir/vue'
import { computed, watch } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useUiStore } from '../../../../stores/ui'
import BaseButton from '../../../ui/BaseButton.vue'
import AISettings from '../AISettings.vue'
import AIProviderList from './AIProviderList.vue'
import AITypeSafePage from './AITypeSafePage.vue'
import { type AIProviderId, providerKind } from './aiProviders'

// 設定の AI タブ（#315）。最初はプロバイダの一覧で、選ぶと 1 段下のそのプロバイダの設定ページが開く。戻る（このボタン・
// Android の戻る）で一覧へ戻る。開いているページは ui.aiProviderPage（Android の戻るがダイアログを閉じずに一覧へ戻せるように、ストアに持つ）。
const ui = useUiStore()

const page = computed(() => ui.aiProviderPage)
const kind = computed(() => (page.value === undefined ? undefined : providerKind(page.value)))

function open(id: AIProviderId): void {
  ui.aiProviderPage = id
}

// 作成画面の「AI で下書き」の初回の案内から ChatGPT へ進むとき（#104）は、ChatGPT のページから始める。流れ自体は AISettings が受け取る。
watch(
  () => ui.aiSetupIntent,
  (intent) => {
    if (intent !== undefined) ui.aiProviderPage = 'chatgpt'
  },
  { immediate: true },
)
</script>

<template>
  <div class="ai-tab">
    <template v-if="page !== undefined && kind !== undefined">
      <div class="head">
        <BaseButton variant="ghost" size="sm" data-testid="ai-provider-back" @click="ui.aiProviderPage = undefined">
          <ArrowLeft class="icon icon-sm" aria-hidden="true" />{{ msg.ai.providerList.back }}
        </BaseButton>
      </div>
      <AITypeSafePage v-if="kind === 'key'" :key="page" />
      <AISettings v-else :key="page" :provider="page" />
    </template>
    <AIProviderList v-else @open="open" />
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
</style>
