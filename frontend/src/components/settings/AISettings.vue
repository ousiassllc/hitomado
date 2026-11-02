<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseSelect from '../ui/BaseSelect.vue'
import AIDestinationBox from './AIDestinationBox.vue'
import { providerLabel } from './aiDestination'
import SettingsInput from './SettingsInput.vue'

const store = useSettingsStore()
const ui = useUiStore()

const providers = Object.entries(msg.ai.providerOptions).map(([value, label]) => ({ value, label }))

const provider = ref('anthropic')
const model = ref('')
const baseUrl = ref('')
const apiKey = ref('')
const errors = ref<{ model?: string; config?: string; key?: string }>({})
const busy = ref<'config' | 'key' | 'clear'>()

const saved = computed(() => store.aiConfig)

function resetDraft(): void {
  provider.value = saved.value?.provider ?? 'anthropic'
  model.value = saved.value?.model ?? ''
  baseUrl.value = saved.value?.baseUrl ?? ''
}
watch(saved, resetDraft, { immediate: true })

const configDirty = computed(
  () =>
    provider.value !== (saved.value?.provider ?? '') ||
    model.value.trim() !== (saved.value?.model ?? '') ||
    (provider.value === 'openai' ? baseUrl.value.trim() : '') !== (saved.value?.baseUrl ?? ''),
)
const providerChanged = computed(() => provider.value !== saved.value?.provider)
const keyProvider = computed(() => saved.value?.provider ?? provider.value)
const consented = computed(() => saved.value?.consentedProviders?.includes(keyProvider.value) ?? false)

async function saveConfig(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  if (model.value.trim() === '') {
    errors.value = { model: msg.ai.modelRequired }
    return
  }
  busy.value = 'config'
  try {
    await store.setAIConfig(
      provider.value,
      model.value.trim(),
      provider.value === 'openai' ? baseUrl.value.trim() : '',
    )
    ui.pushToast('success', msg.ai.configSaved)
  } catch (e) {
    errors.value = { config: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

async function saveKey(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  if (apiKey.value === '') {
    errors.value = { key: msg.ai.apiKeyRequired }
    return
  }
  busy.value = 'key'
  try {
    await store.setAIApiKey(keyProvider.value, apiKey.value)
    apiKey.value = ''
    ui.pushToast('success', msg.ai.apiKeySaved)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

async function clearKey(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'clear'
  try {
    await store.clearAIApiKey(keyProvider.value)
    apiKey.value = ''
    ui.pushToast('success', msg.ai.apiKeyCleared)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}
</script>

<template>
  <section class="ai" :aria-label="msg.ai.heading">
    <h3 class="heading">{{ msg.ai.heading }}</h3>
    <p class="intro">{{ msg.ai.intro }}</p>

    <AIDestinationBox
      :provider="saved?.provider ?? provider"
      :base-url="saved?.baseUrl"
      :draft-provider="provider"
      :draft-base-url="baseUrl"
    />

    <div class="form">
      <div class="field">
        <span class="label">{{ msg.ai.provider }}</span>
        <BaseSelect v-model="provider" :options="providers" :ariaLabel="msg.ai.provider" />
      </div>
      <SettingsInput
        v-model="model"
        :label="msg.ai.model"
        :placeholder="msg.ai.modelPlaceholder"
        :error="errors.model"
        @enter="saveConfig"
      />
      <SettingsInput
        v-if="provider === 'openai'"
        v-model="baseUrl"
        type="url"
        :label="msg.ai.baseUrl"
        :placeholder="msg.ai.baseUrlPlaceholder"
        :hint="msg.ai.baseUrlHint"
        @enter="saveConfig"
      />
      <p v-if="errors.config" class="error" role="alert">{{ errors.config }}</p>
      <div class="actions">
        <BaseButton
          variant="primary"
          :disabled="!configDirty"
          :loading="busy === 'config'"
          @click="saveConfig"
        >
          {{ msg.ai.saveConfig }}
        </BaseButton>
      </div>
    </div>

    <div class="key">
      <h4 class="sub">{{ msg.ai.apiKeyHeading }}</h4>
      <p class="status">
        {{ providerLabel(keyProvider) }}：
        <strong>{{ saved?.hasApiKey ? msg.ai.apiKeyStatusSet : msg.ai.apiKeyStatusUnset }}</strong>
      </p>
      <p v-if="providerChanged" class="hint">{{ msg.ai.apiKeyNeedsSavedProvider }}</p>
      <SettingsInput
        v-model="apiKey"
        type="password"
        autocomplete="new-password"
        :label="msg.ai.apiKeyFor(providerLabel(keyProvider))"
        :placeholder="msg.ai.apiKeyPlaceholder"
        :hint="msg.ai.apiKeyStored"
        :error="errors.key"
        :disabled="providerChanged"
        @enter="saveKey"
      />
      <div class="actions">
        <BaseButton
          variant="danger"
          :disabled="!saved?.hasApiKey || busy !== undefined"
          :loading="busy === 'clear'"
          @click="clearKey"
        >
          {{ msg.ai.apiKeyClear }}
        </BaseButton>
        <BaseButton
          variant="primary"
          :disabled="providerChanged || apiKey === ''"
          :loading="busy === 'key'"
          @click="saveKey"
        >
          {{ msg.ai.apiKeySave }}
        </BaseButton>
      </div>
    </div>

    <div class="consent">
      <h4 class="sub">{{ msg.ai.consentHeading }}</h4>
      <p class="status" role="status">
        {{ consented ? msg.ai.consentGiven(providerLabel(keyProvider)) : msg.ai.consentNone(providerLabel(keyProvider)) }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.ai {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.heading,
.sub {
  margin: 0;
}

.heading {
  font-size: var(--font-size-lg);
}

.sub {
  font-size: var(--font-size-md);
}

.intro,
.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.form,
.key,
.consent {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  max-width: 320px;
}

.label {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
}

.error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
