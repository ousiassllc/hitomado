<script setup lang="ts">
import { ref } from 'vue'
import { settings as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import SettingsInput from '../SettingsInput.vue'

// API キーの登録・削除（anthropic／openai）。キーは入力欄にだけ置き、保存後は消す。再表示しない。
const props = defineProps<{
  providerLabel: string
  hasKey: boolean
  busy?: 'config' | 'key' | 'clear' | 'models' | 'consent'
  error?: string
}>()
const emit = defineEmits<{ save: [apiKey: string]; clear: [] }>()

const apiKey = ref('')
const localError = ref<string>()

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
    <h4 class="sub">{{ msg.ai.apiKeyHeading }}</h4>
    <p class="status">
      {{ props.providerLabel }}：
      <strong>{{ props.hasKey ? msg.ai.apiKeyStatusSet : msg.ai.apiKeyStatusUnset }}</strong>
    </p>
    <SettingsInput
      v-model="apiKey"
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
        @click="emit('clear')"
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
  </div>
</template>

<style scoped>
.key {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.sub {
  margin: 0;
  font-size: var(--font-size-md);
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
