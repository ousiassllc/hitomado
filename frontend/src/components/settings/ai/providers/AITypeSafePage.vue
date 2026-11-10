<script setup lang="ts">
import { computed, ref } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import AIApiKey from '../AIApiKey.vue'
import AIDestinationBox from '../AIDestinationBox.vue'
import { hasProviderCredential } from './aiProviders'

// TypeSafe AI（Jev）の設定ページ（#314・#315）。文章を書かないモデルなので、モデルの選択・エフォートは無く、
// 「AI で下書き」の既定のプロバイダにもならない。ここでは API キーだけを登録・削除する（キーは資格情報ストア）。
const PROVIDER = 'typesafe'
const store = useSettingsStore()
const ui = useUiStore()

const keyForm = ref<InstanceType<typeof AIApiKey>>()
const busy = ref<'key' | 'clear'>()
const error = ref<string>()
const label = msg.ai.providerOptions[PROVIDER] ?? PROVIDER
const hasKey = computed(() => hasProviderCredential(PROVIDER, store.aiConfig))

async function saveKey(apiKey: string): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'key'
  try {
    await store.setAIApiKey(PROVIDER, apiKey)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeySaved)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
  }
}

async function clearKey(): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'clear'
  try {
    await store.clearAIApiKey(PROVIDER)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeyCleared)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
  }
}
</script>

<template>
  <section class="ai" :aria-label="label" data-testid="ai-typesafe-page">
    <div class="group">
      <h3 class="heading">{{ label }}</h3>
      <p class="intro">{{ msg.ai.typesafe.note }}</p>
    </div>
    <AIDestinationBox :provider="PROVIDER" :draft-provider="PROVIDER" draft-base-url="" />
    <AIApiKey
      ref="keyForm"
      :provider-label="label"
      :has-key="hasKey"
      :busy="busy"
      :error="error"
      @save="saveKey"
      @clear="clearKey"
    />
  </section>
</template>

<style scoped>
.ai {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
