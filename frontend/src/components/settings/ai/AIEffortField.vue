<script setup lang="ts">
import { computed } from 'vue'
import { settings as msg } from '../../../lib/messages'
import BaseSelect, { type SelectOption } from '../../ui/BaseSelect.vue'

// 「エフォート」（モデルの reasoning effort）の選択（F-08。#111）。「AI で下書き」の行に、モデルの選択の横（390px 幅では下）に置く。
// 先頭は常に「モデルの既定」（何も送らない）。出す段階は呼び出し側が決める。空ならエフォートの設定がないモデルなので、選択自体を出さない。
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
  </div>
</template>

<style scoped>
.field {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.label {
  flex: none;
  color: var(--mute);
  font-size: var(--font-size-xs);
  white-space: nowrap;
}

/* 390px 幅: ラベルを上に置き、選択欄は横いっぱい。 */
@media (max-width: 640px) {
  .field {
    flex-direction: column;
    align-items: stretch;
  }

  .label {
    color: var(--ink);
    font-size: var(--font-size-sm);
  }
}
</style>
