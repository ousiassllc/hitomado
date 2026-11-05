<script setup lang="ts">
import { computed } from 'vue'
import { settings as msg } from '../../../lib/messages'
import BaseSelect, { type SelectOption } from '../../ui/BaseSelect.vue'

// 「エフォート」（モデルの reasoning effort）の選択（F-08。#111）。先頭は常に「モデルの既定」（何も送らない）。
// 出す段階は呼び出し側が決める。空ならエフォートの設定がないモデルなので、選択自体を出さない。
const props = defineProps<{
  /** 出す段階（弱い順）。 */
  efforts: string[]
  /** 選択中の段階。空は「モデルの既定」。 */
  modelValue: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

// reka-ui の SelectItem は空文字を値にできないので、「モデルの既定」はこの値で表す。
const DEFAULT = 'default'

const options = computed<SelectOption[]>(() => [
  { value: DEFAULT, label: msg.ai.effortDefault },
  ...props.efforts.map((e) => ({ value: e, label: msg.ai.effortLabels[e] ?? e })),
])
</script>

<template>
  <div v-if="efforts.length > 0" class="field" data-testid="ai-effort">
    <span class="label">{{ msg.ai.effort }}</span>
    <BaseSelect
      :model-value="modelValue === '' ? DEFAULT : modelValue"
      :options="options"
      :ariaLabel="msg.ai.effort"
      @update:model-value="(v) => emit('update:modelValue', v === DEFAULT ? '' : v)"
    />
    <p class="hint">{{ msg.ai.effortHint }}</p>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
  max-width: 320px;
}

.label {
  font-size: var(--font-size-sm);
}

.hint {
  margin: calc(var(--gap-help) - var(--gap-label)) 0 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
