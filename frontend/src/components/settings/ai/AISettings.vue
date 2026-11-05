<script setup lang="ts">
import { ref, watch } from 'vue'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import BaseSelect from '../../ui/BaseSelect.vue'
import SettingsInput from '../SettingsInput.vue'
import AIApiKey from './AIApiKey.vue'
import AIChatGPTLogin from './AIChatGPTLogin.vue'
import AIConsentSection from './AIConsentSection.vue'
import AIDestinationBox from './AIDestinationBox.vue'
import AIEffortField from './AIEffortField.vue'
import AIModelField from './AIModelField.vue'
import { DEFAULT_AI_PROVIDER, providerLabel } from './aiDestination'
import { useAiDraftState } from './useAiDraftState'
import { type AiBusy, useAiSetupFlow } from './useAiSetupFlow'

// プロバイダを選ぶ → (未同意なら) 同意の画面 → 同意でそのままモデル一覧を取得 → 一覧からモデルを選んで保存、という 1 つの流れ（#105）。
// 作成画面の「AI で下書き」の初回の案内から ChatGPT へ進むとき（#104）も、この流れの入口（`ui.aiSetupIntent`）から入る。
// 一覧の取得・同意の確認は保存前の選択（プロバイダ・Base URL）に対して行い、同意前の送信先へ送らないのは Go が決める（reason=consent_required）。
const store = useSettingsStore()
const ui = useUiStore()

const providers = Object.entries(msg.ai.providerOptions).map(([value, label]) => ({ value, label }))

const provider = ref(DEFAULT_AI_PROVIDER)
const model = ref('')
const baseUrl = ref('')
const effort = ref('')
const errors = ref<{ model?: string; config?: string; key?: string; consent?: string }>({})
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const chatgptLogin = ref<InstanceType<typeof AIChatGPTLogin>>()
const busy = ref<AiBusy>()
const consentOpen = ref(false)

const {
  saved,
  isChatGPT,
  draftBaseUrl,
  configDirty,
  hasCredential,
  models,
  efforts,
  fitEffort,
  draftConsent,
  draftDestination,
  consentText,
  destinationDecided,
} = useAiDraftState(provider, model, baseUrl, effort)

const { refresh, agree, declineConsent } = useAiSetupFlow({
  provider,
  draftBaseUrl,
  isChatGPT,
  busy,
  errors,
  consentOpen,
  keyForm,
  chatgptLogin,
})

function onProvider(v: string): void {
  provider.value = v
  errors.value = {}
  keyForm.value?.reset()
  fitEffort()
  if (destinationDecided.value) void refresh()
}

function onModel(v: string): void {
  model.value = v
  fitEffort()
}

async function saveConfig(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  if (model.value.trim() === '') {
    errors.value = { model: msg.ai.modelRequired }
    return
  }
  busy.value = 'config'
  try {
    await store.setAIConfig(provider.value, model.value.trim(), draftBaseUrl.value, effort.value)
    ui.pushToast('success', msg.ai.configSaved)
  } catch (e) {
    errors.value = { config: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

async function saveKey(apiKey: string): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'key'
  try {
    await store.setAIApiKey(provider.value, apiKey)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeySaved)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
    return
  } finally {
    busy.value = undefined
  }
  await refresh() // キーの登録・変更のあとに取得する
}

async function clearKey(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'clear'
  try {
    await store.clearAIApiKey(provider.value)
    keyForm.value?.reset()
    ui.pushToast('success', msg.ai.apiKeyCleared)
  } catch (e) {
    errors.value = { key: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

// 作成画面の「AI で下書き」の初回の案内から「Continue with ChatGPT」で来たとき: ChatGPT を選んで流れを始める（ログインまで進む）。
function startFromIntent(): void {
  if (saved.value === undefined || ui.aiSetupIntent === undefined) return
  ui.takeAiSetupIntent()
  provider.value = 'chatgpt'
  void refresh(true)
}
watch(() => [saved.value !== undefined, ui.aiSetupIntent], startFromIntent, { immediate: true })
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
        <BaseSelect
          :model-value="provider"
          :options="providers"
          :ariaLabel="msg.ai.provider"
          @update:model-value="onProvider"
        />
      </div>
      <SettingsInput
        v-if="provider === 'openai'"
        v-model="baseUrl"
        type="url"
        :label="msg.ai.baseUrl"
        :placeholder="msg.ai.baseUrlPlaceholder"
        :hint="msg.ai.baseUrlHint"
        @enter="refresh()"
      />
      <p v-if="provider === 'openai' && models === undefined" class="hint">{{ msg.ai.baseUrlDecide }}</p>
      <AIModelField
        :model-value="model"
        :models="models"
        :loading="busy === 'models'"
        :can-refresh="busy === undefined || busy === 'models'"
        :error="errors.model"
        @update:model-value="onModel"
        @refresh="refresh()"
        @enter="saveConfig"
      />
      <AIEffortField v-model="effort" :efforts="efforts" />
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
      ref="chatgptLogin"
      :signed-in="saved?.chatgptSignedIn ?? false"
      :email="saved?.chatgptEmail ?? ''"
      :accounts="saved?.chatgptAccounts ?? []"
      @account-changed="refresh()"
    />

    <AIApiKey
      v-if="!isChatGPT"
      ref="keyForm"
      :provider-label="providerLabel(provider)"
      :has-key="hasCredential"
      :busy="busy"
      :error="errors.key"
      @save="saveKey"
      @clear="clearKey"
    />

    <AIConsentSection
      :text="consentText"
      :error="errors.consent"
      :needed="draftConsent !== true"
      :disabled="busy !== undefined"
      :destination="draftDestination"
      :open="consentOpen"
      :saving="busy === 'consent'"
      @ask="consentOpen = true"
      @agree="agree"
      @decline="declineConsent"
    />
  </section>
</template>

<style scoped>
.ai {
  display: flex;
  flex-direction: column;
  gap: var(--s-4);
}

.heading {
  margin: 0;
  font-size: var(--font-size-base);
}

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.form {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
  max-width: 320px;
}

.label {
  font-size: var(--font-size-sm);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
