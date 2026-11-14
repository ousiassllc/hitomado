<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import SettingsInput from '../SettingsInput.vue'
import AIApiKey from './AIApiKey.vue'
import AIChatGPTLogin from './AIChatGPTLogin.vue'
import AIConsentSection from './AIConsentSection.vue'
import AIDestinationBox from './AIDestinationBox.vue'
import { providerLabel } from './aiDestination'
import { type AiBusy, useAiConnect } from './useAiConnect'

// 1 つのプロバイダの「つなぎ先」のページ（#315・#376。「AI のつなぎ先」の一覧から開く）。ここで行うのは、つなぐ操作
// （ChatGPT のログイン・API キーの登録・OpenAI 互換の Base URL・送信への同意）だけで、モデルは選ばない（「機能ごとに使う AI」で選ぶ）。
// 「ログイン」「API キーを追加」から来たとき（ui.aiConnectStart）は、つなぐ流れ（未同意なら同意の画面 → ログイン・キーの入力）を始める。
// OpenAI 互換だけは、Base URL を決めてから同意するので、まず Base URL の入力へ誘導する。
const store = useSettingsStore()
const ui = useUiStore()

const props = defineProps<{ provider: string }>()
const provider = ref(props.provider)

const errors = ref<{ key?: string; consent?: string; baseUrl?: string }>({})
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const chatgptLogin = ref<InstanceType<typeof AIChatGPTLogin>>()
const busy = ref<AiBusy>()
const consentOpen = ref(false)
const baseUrlInput = ref('')

const c = useAiConnect({ provider, busy, errors, consentOpen, keyForm, chatgptLogin })

const isOpenAI = provider.value === 'openai'
const baseUrlDirty = () => baseUrlInput.value.trim() !== c.baseUrl.value

async function saveBaseUrl(): Promise<void> {
  if (busy.value || !baseUrlDirty()) return
  errors.value = {}
  busy.value = 'baseUrl'
  try {
    await store.setAIBaseURL(baseUrlInput.value.trim())
    ui.pushToast('success', msg.ai.connect.baseUrlSaved)
  } catch (e) {
    errors.value = { baseUrl: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

const baseUrlField = ref<HTMLElement>()
onMounted(() => {
  baseUrlInput.value = c.baseUrl.value
  if (!ui.takeAiConnectStart()) return
  if (isOpenAI) baseUrlField.value?.querySelector('input')?.focus()
  else c.begin()
})
</script>

<template>
  <section class="ai" :aria-label="providerLabel(provider)">
    <div class="group">
      <h3 class="heading">{{ providerLabel(provider) }}</h3>
      <p class="intro">{{ msg.ai.connect.pageIntro }}</p>
    </div>

    <AIDestinationBox
      :provider="provider"
      :base-url="c.baseUrl.value"
      :draft-provider="provider"
      :draft-base-url="isOpenAI ? baseUrlInput.trim() : ''"
    />

    <div v-if="isOpenAI" ref="baseUrlField" class="form">
      <SettingsInput
        v-model="baseUrlInput"
        mono
        type="url"
        :label="msg.ai.baseUrl"
        :placeholder="msg.ai.baseUrlPlaceholder"
        :hint="msg.ai.baseUrlHint"
        :error="errors.baseUrl"
        @enter="saveBaseUrl"
      />
      <div class="actions">
        <BaseButton
          variant="secondary"
          :disabled="!baseUrlDirty()"
          :loading="busy === 'baseUrl'"
          data-testid="ai-base-url-save"
          @click="saveBaseUrl"
        >
          {{ msg.ai.connect.baseUrlSave }}
        </BaseButton>
      </div>
    </div>

    <AIChatGPTLogin
      v-if="c.isChatGPT.value"
      ref="chatgptLogin"
      :signed-in="c.cfg.value?.chatgptSignedIn ?? false"
      :email="c.cfg.value?.chatgptEmail ?? ''"
      :accounts="c.cfg.value?.chatgptAccounts ?? []"
    />

    <AIApiKey
      v-else
      ref="keyForm"
      :provider-label="providerLabel(provider)"
      :has-key="c.hasCredential.value"
      :busy="busy"
      :error="errors.key"
      @save="c.saveKey"
      @clear="c.clearKey"
    />

    <AIConsentSection
      :text="c.consentText.value"
      :error="errors.consent"
      :needed="!c.consented.value"
      :disabled="busy !== undefined || (isOpenAI && baseUrlDirty())"
      :destination="c.destination.value"
      :open="consentOpen"
      :saving="busy === 'consent'"
      @ask="consentOpen = true"
      @agree="c.agree"
      @decline="c.declineConsent"
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

.form {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
