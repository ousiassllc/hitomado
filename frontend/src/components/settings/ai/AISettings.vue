<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import BaseSelect from '../../ui/BaseSelect.vue'
import SettingsInput from '../SettingsInput.vue'
import AIApiKey from './AIApiKey.vue'
import AIChatGPTLogin from './AIChatGPTLogin.vue'
import AIDestinationBox from './AIDestinationBox.vue'
import AIModelField from './AIModelField.vue'
import { aiDestination, providerLabel } from './aiDestination'

const store = useSettingsStore()
const ui = useUiStore()

const providers = Object.entries(msg.ai.providerOptions).map(([value, label]) => ({ value, label }))

const provider = ref('anthropic')
const model = ref('')
const baseUrl = ref('')
const errors = ref<{ model?: string; config?: string; key?: string; consent?: string }>({})
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const busy = ref<'config' | 'key' | 'clear' | 'models' | 'consent'>()

const saved = computed(() => store.aiConfig)

function resetDraft(): void {
  provider.value = saved.value?.provider ?? 'anthropic'
  model.value = saved.value?.model ?? ''
  baseUrl.value = saved.value?.baseUrl ?? ''
}
watch(saved, resetDraft, { immediate: true })

const isChatGPT = computed(() => provider.value === 'chatgpt')
const draftBaseUrl = computed(() => (provider.value === 'openai' ? baseUrl.value.trim() : ''))
const baseUrlChanged = computed(() => draftBaseUrl.value !== (saved.value?.baseUrl ?? ''))
const configDirty = computed(
  () =>
    provider.value !== (saved.value?.provider ?? '') ||
    model.value.trim() !== (saved.value?.model ?? '') ||
    baseUrlChanged.value,
)
const providerChanged = computed(() => provider.value !== saved.value?.provider)
const keyProvider = computed(() => saved.value?.provider ?? provider.value)
const consented = computed(() => saved.value?.consentedProviders?.includes(keyProvider.value) ?? false)

// モデル一覧は保存済みの設定（プロバイダ・Base URL）で取得する。編集中の値と食い違うときは取得を促さない。
const modelsInSync = computed(() => saved.value !== undefined && !providerChanged.value && !baseUrlChanged.value)
const models = computed(() => (modelsInSync.value ? store.aiModels[keyProvider.value] : undefined))

/** モデル一覧を取得する。取得できないときは理由が一覧の注記に出て、手入力のままになる（エラーにはしない）。 */
async function loadModels(): Promise<void> {
  if (busy.value && busy.value !== 'models') return
  busy.value = 'models'
  try {
    await store.loadAIModels(keyProvider.value)
  } catch (e) {
    errors.value = { ...errors.value, model: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

async function saveConfig(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  if (model.value.trim() === '') {
    errors.value = { model: msg.ai.modelRequired }
    return
  }
  const destChanged = providerChanged.value || baseUrlChanged.value
  busy.value = 'config'
  try {
    await store.setAIConfig(provider.value, model.value.trim(), draftBaseUrl.value)
    ui.pushToast('success', msg.ai.configSaved)
  } catch (e) {
    errors.value = { config: parseBindingError(e).message }
    return
  } finally {
    busy.value = undefined
  }
  // 送信先（プロバイダ・Base URL）を変えたあとは一覧を取り直す。
  if (destChanged) await loadModels()
}

async function saveKey(apiKey: string): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'key'
  try {
    await store.setAIApiKey(keyProvider.value, apiKey)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeySaved)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
    return
  } finally {
    busy.value = undefined
  }
  await loadModels() // キーの登録・変更のあとに取得する
}

async function clearKey(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'clear'
  try {
    await store.clearAIApiKey(keyProvider.value)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeyCleared)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

/** 設定画面での同意。同意前の送信先には一覧の取得も送らないので、同意したらそのまま一覧を取得する。 */
async function agreeNow(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'consent'
  try {
    await store.recordAIConsent(keyProvider.value)
  } catch (e) {
    errors.value = { consent: parseBindingError(e).message }
    return
  } finally {
    busy.value = undefined
  }
  await loadModels()
}

const savedDestination = computed(() => aiDestination(keyProvider.value, saved.value?.baseUrl))
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
        v-if="provider === 'openai'"
        v-model="baseUrl"
        type="url"
        :label="msg.ai.baseUrl"
        :placeholder="msg.ai.baseUrlPlaceholder"
        :hint="msg.ai.baseUrlHint"
        @enter="saveConfig"
      />
      <AIModelField
        v-model="model"
        :models="models"
        :loading="busy === 'models'"
        :can-refresh="modelsInSync && (busy === undefined || busy === 'models')"
        :error="errors.model"
        @refresh="loadModels"
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

    <AIChatGPTLogin
      v-if="isChatGPT"
      :signed-in="saved?.chatgptSignedIn ?? false"
      :email="saved?.chatgptEmail ?? ''"
      :accounts="saved?.chatgptAccounts ?? []"
      @account-changed="saved?.provider === 'chatgpt' && loadModels()"
    />
    <p v-if="isChatGPT && providerChanged" class="hint">{{ msg.ai.chatgpt.saveBeforeUse }}</p>

    <AIApiKey
      v-if="!isChatGPT"
      ref="keyForm"
      :provider-label="providerLabel(keyProvider)"
      :has-key="saved?.hasApiKey ?? false"
      :provider-changed="providerChanged"
      :busy="busy"
      :error="errors.key"
      @save="saveKey"
      @clear="clearKey"
    />

    <div class="consent">
      <h4 class="sub">{{ msg.ai.consentHeading }}</h4>
      <p class="status" role="status">
        {{ consented ? msg.ai.consentGiven(providerLabel(keyProvider)) : msg.ai.consentNone(providerLabel(keyProvider)) }}
      </p>
      <p v-if="errors.consent" class="error" role="alert">{{ errors.consent }}</p>
      <div v-if="!consented && saved" class="actions">
        <BaseButton
          variant="secondary"
          :disabled="busy !== undefined || providerChanged || baseUrlChanged"
          :loading="busy === 'consent'"
          data-testid="ai-consent-now"
          @click="agreeNow"
        >
          {{ msg.ai.consentAgreeNow(msg.ai.destination(savedDestination.providerLabel, savedDestination.url)) }}
        </BaseButton>
      </div>
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
