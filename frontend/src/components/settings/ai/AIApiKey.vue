<script setup lang="ts">
import { ref } from 'vue'
import { settings as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import ConfirmDialog from '../../ui/ConfirmDialog.vue'
import SettingsInput from '../SettingsInput.vue'

// API キーの登録・削除（anthropic／openai）。キーは入力欄にだけ置き、保存後は消す。再表示しない。
const props = defineProps<{
  providerLabel: string
  hasKey: boolean
  busy?: 'key' | 'clear' | 'consent' | 'baseUrl'
  error?: string
}>()
const emit = defineEmits<{ save: [apiKey: string]; clear: [] }>()

const apiKey = ref('')
const localError = ref<string>()
/** キーの削除の確認（ガイド O2）。確認したら閉じ、失敗は `error` に出る。 */
const clearOpen = ref(false)

function clear(): void {
  clearOpen.value = false
  emit('clear')
}

function save(): void {
  localError.value = undefined
  if (apiKey.value === '') {
    localError.value = msg.ai.apiKeyRequired
    return
  }
  emit('save', apiKey.value)
}

const root = ref<HTMLElement>()
defineExpose({
  reset: () => (apiKey.value = ''),
  /** キーが未登録のとき、入力欄へ誘導する（取得にはキーが要る）。 */
  focus: () => root.value?.querySelector('input')?.focus(),
})
</script>

<template>
  <div ref="root" class="key">
    <div class="group">
      <h4 class="sub">{{ msg.ai.apiKeyHeading }}</h4>
      <p class="status">
        {{ props.providerLabel }}：
        <strong>{{ props.hasKey ? msg.ai.apiKeyStatusSet : msg.ai.apiKeyStatusUnset }}</strong>
      </p>
    </div>
    <SettingsInput
      v-model="apiKey"
      mono
      type="password"
      autocomplete="new-password"
      :label="msg.ai.apiKeyFor(props.providerLabel)"
      :placeholder="msg.ai.apiKeyPlaceholder"
      :hint="msg.ai.apiKeyStored"
      :error="props.error ?? localError"
      @enter="save"
    />
    <div class="actions">
      <BaseButton
        variant="danger"
        :disabled="!props.hasKey || props.busy !== undefined"
        :loading="props.busy === 'clear'"
        @click="clearOpen = true"
      >
        {{ msg.ai.apiKeyClear }}
      </BaseButton>
      <BaseButton
        variant="primary"
        :disabled="apiKey === ''"
        :loading="props.busy === 'key'"
        @click="save"
      >
        {{ msg.ai.apiKeySave }}
      </BaseButton>
    </div>
    <ConfirmDialog
      v-model:open="clearOpen"
      :title="msg.ai.apiKeyClearTitle(props.providerLabel)"
      :detail="msg.ai.apiKeyClearDetail"
      :confirm-label="msg.ai.apiKeyClearConfirm"
      @confirm="clear"
    />
  </div>
</template>

<style scoped>
.key {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

/* 見出しとその状態文の組。組の中は --s-2、組どうしは親の gap（--gap-field）。 */
.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sub {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
