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
  }>(),
  {
    type: 'text',
    placeholder: undefined,
    hint: undefined,
    error: undefined,
    disabled: false,
    autocomplete: 'off',
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
  gap: var(--space-1);
  min-width: 0;
}

.field-label {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.field-input {
  min-height: 32px;
  padding: 0 var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: var(--font-size-md);
}

.field-input:disabled {
  opacity: 0.55;
}

.field-input[aria-invalid="true"] {
  border-color: var(--color-danger);
}

.field-hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.field-error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}
</style>
