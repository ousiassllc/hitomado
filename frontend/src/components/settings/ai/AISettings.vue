<script setup lang="ts">
import { ref, watch } from 'vue'
import type { AIModelList } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
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
import AIModelField from './AIModelField.vue'
import { DEFAULT_AI_PROVIDER, providerLabel } from './aiDestination'
import { useAiDraftState } from './useAiDraftState'

// プロバイダを選ぶ → (未同意なら) 同意の画面 → 同意でそのままモデル一覧を取得 → 一覧からモデルを選んで保存、という 1 つの流れ（#105）。
// 作成画面の「AI で下書き」の初回の案内から ChatGPT へ進むとき（#104）も、この流れの入口（`ui.aiSetupIntent`）から入る。
// 一覧の取得・同意の確認は保存前の選択（プロバイダ・Base URL）に対して行い、同意前の送信先へ送らないのは Go が決める（reason=consent_required）。
const store = useSettingsStore()
const ui = useUiStore()

const providers = Object.entries(msg.ai.providerOptions).map(([value, label]) => ({ value, label }))

const provider = ref(DEFAULT_AI_PROVIDER)
const model = ref('')
const baseUrl = ref('')
const errors = ref<{ model?: string; config?: string; key?: string; consent?: string }>({})
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const chatgptLogin = ref<InstanceType<typeof AIChatGPTLogin>>()
const busy = ref<'config' | 'key' | 'clear' | 'models' | 'consent'>()
const consentOpen = ref(false)

const {
  saved,
  isChatGPT,
  draftBaseUrl,
  configDirty,
  hasCredential,
  models,
  draftConsent,
  draftDestination,
  consentText,
  destinationDecided,
} = useAiDraftState(provider, model, baseUrl)

/** 選んだ送信先のモデル一覧を取得する。取得できないときは理由が一覧の注記に出て、手入力のままになる（エラーにはしない）。 */
async function loadModels(): Promise<AIModelList | undefined> {
  if (busy.value && busy.value !== 'models') return undefined
  busy.value = 'models'
  try {
    return await store.loadAIModels(provider.value, draftBaseUrl.value)
  } catch (e) {
    errors.value = { ...errors.value, model: parseBindingError(e).message }
    return undefined
  } finally {
    busy.value = undefined
  }
}

/**
 * 取得のあとの案内。資格情報が無いとき、ChatGPT は「Continue with ChatGPT」のログインへ進み（login=true のとき自動で始める）、
 * API キーのプロバイダはキーの入力欄へ誘導する。
 */
function guideCredentials(r: AIModelList, login: boolean): void {
  if (r.reason !== 'no_credentials') return
  if (isChatGPT.value) {
    if (login) void chatgptLogin.value?.login()
    return
  }
  keyForm.value?.focus()
}

/**
 * プロバイダ・送信先を選んだときの流れの入口。まず取得を試し、同意前の送信先なら（Go が取得を送らず consent_required を返すので）
 * 同意の画面を出す。同意済みならそのまま一覧が出る。login は、資格情報が無い ChatGPT のログインを自動で始めるか。
 */
async function refresh(login = false): Promise<void> {
  const [p, url] = [provider.value, draftBaseUrl.value]
  const r = await loadModels()
  if (!r || p !== provider.value || url !== draftBaseUrl.value) return // 取得中に選び直された
  if (r.reason === 'consent_required') {
    consentOpen.value = true
    return
  }
  guideCredentials(r, login)
}

function onProvider(v: string): void {
  provider.value = v
  errors.value = {}
  keyForm.value?.reset()
  if (destinationDecided.value) void refresh()
}

/** 同意の画面の「同意する」。同意を保存し、すぐ一覧を取得する。ChatGPT で未ログインなら、続けてログインへ進む。 */
async function agree(): Promise<void> {
  if (busy.value) return
  errors.value = {}
  busy.value = 'consent'
  const [p, url] = [provider.value, draftBaseUrl.value]
  try {
    await store.recordAIConsent(p, url)
  } catch (e) {
    errors.value = { consent: parseBindingError(e).message }
    consentOpen.value = false
    return
  } finally {
    busy.value = undefined
  }
  consentOpen.value = false
  const r = await loadModels()
  if (!r || p !== provider.value || url !== draftBaseUrl.value) return
  guideCredentials(r, true)
}

/** 閉じた（同意しなかった）とき。取得はせず、プロバイダの選択だけが残る（モデル名は手入力できる）。 */
function declineConsent(): void {
  consentOpen.value = false
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
    await store.setAIConfig(provider.value, model.value.trim(), draftBaseUrl.value)
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
        v-model="model"
        :models="models"
        :loading="busy === 'models'"
        :can-refresh="busy === undefined || busy === 'models'"
        :error="errors.model"
        @refresh="refresh()"
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
  gap: var(--space-4);
}

.heading {
  margin: 0;
  font-size: var(--font-size-lg);
}

.intro,
.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.form {
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
