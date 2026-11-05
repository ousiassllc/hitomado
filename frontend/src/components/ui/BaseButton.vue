<script setup lang="ts">
import Spinner from './Spinner.vue'

withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'secondary', loading: false, disabled: false, type: 'button' },
)
</script>

<template>
  <button
    class="btn"
    :class="`btn-${variant}`"
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
  >
    <Spinner v-if="loading" :size="14" />
    <slot />
  </button>
</template>

<style scoped>
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  min-height: 32px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: background var(--transition-fast);
  white-space: nowrap;
}

.btn:hover:not(:disabled) {
  background: var(--color-selected);
}

/* 無効（押せない）ボタンだけを薄くする。送信中など loading のボタンは文字を読ませたいので、色はそのままにする。 */
.btn:disabled:not([aria-busy='true']) {
  opacity: 0.55;
  cursor: not-allowed;
}

.btn[aria-busy='true'] {
  cursor: progress;
}

.btn-primary {
  background: var(--color-accent);
  border-color: var(--color-accent);
  color: var(--color-accent-text);
}

.btn-primary:hover:not(:disabled) {
  background: var(--color-accent);
  filter: brightness(1.1);
}

.btn-danger {
  background: var(--color-danger);
  border-color: var(--color-danger);
  color: var(--color-danger-text);
}

.btn-danger:hover:not(:disabled) {
  background: var(--color-danger);
  filter: brightness(1.1);
}

.btn-ghost {
  background: transparent;
  border-color: transparent;
}
</style>
