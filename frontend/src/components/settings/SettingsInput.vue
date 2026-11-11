<script setup lang="ts">
import { useId } from 'vue'

withDefaults(
  defineProps<{
    modelValue: string
    label: string
    type?: 'text' | 'password' | 'url'
    placeholder?: string
    hint?: string
    error?: string
    disabled?: boolean
    autocomplete?: string
    mono?: boolean
  }>(),
  {
    type: 'text',
    placeholder: undefined,
    hint: undefined,
    error: undefined,
    disabled: false,
    autocomplete: 'off',
    mono: false,
  },
)

defineEmits<{ 'update:modelValue': [value: string]; enter: [] }>()

const id = useId()
</script>

<template>
  <div class="field">
    <label class="field-label" :for="id">{{ label }}</label>
    <input
      :id="id"
      class="field-input"
      :class="{ mono }"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :autocomplete="autocomplete"
      :aria-invalid="error ? true : undefined"
      :aria-describedby="error ? `${id}-err` : hint ? `${id}-hint` : undefined"
      spellcheck="false"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      @keydown.enter.prevent="$emit('enter')"
    />
    <p v-if="hint && !error" :id="`${id}-hint`" class="field-hint">{{ hint }}</p>
    <p v-if="error" :id="`${id}-err`" class="field-error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
  min-width: 0;
}

.field-label {
  font-size: var(--font-size-sm);
}

.field-input {
  min-height: var(--control-h);
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.field-input.mono {
  font-family: var(--font-mono);
}

/* 識別子欄でも説明文の placeholder は文章なので sans に戻す */
.field-input.mono::placeholder {
  font-family: var(--font-sans);
}

.field-input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.field-input:disabled {
  opacity: 0.5;
}

.field-input[aria-invalid="true"] {
  border-color: var(--danger);
}

/* 640px 以下は 16px（iOS・Android のフォーカス時の自動ズームを避ける） */
@media (max-width: 640px) {
  .field-input {
    font-size: var(--font-size-base);
  }
}

.field-hint {
  margin: calc(var(--gap-help) - var(--gap-label)) 0 0;
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.field-error {
  margin: calc(var(--gap-help) - var(--gap-label)) 0 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
