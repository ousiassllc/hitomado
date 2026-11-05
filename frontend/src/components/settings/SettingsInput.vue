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
  gap: var(--s-1);
  min-width: 0;
}

.field-label {
  font-size: var(--font-size-sm);
}

.field-input {
  min-height: var(--control-h);
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.field-input:focus {
  border-color: var(--focus);
  box-shadow: 0 0 0 2px var(--ring);
  outline: none;
}

.field-input:disabled {
  opacity: 0.5;
}

.field-input[aria-invalid="true"] {
  border-color: var(--danger);
}

.field-hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.field-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
}
</style>
