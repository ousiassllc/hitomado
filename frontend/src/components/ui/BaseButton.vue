<script setup lang="ts">
import { vRipple } from '../../lib/ripple'
import Spinner from './Spinner.vue'

withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    /** md = 高さ 40・16px。sm = 高さ 32・14px（ツールバーや行の中の小さい操作） */
    size?: 'md' | 'sm'
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'secondary', size: 'md', loading: false, disabled: false, type: 'button' },
)
</script>

<template>
  <button
    v-ripple
    class="btn"
    :class="[`btn-${variant}`, `btn-${size}`]"
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
  >
    <Spinner v-if="loading" />
    <slot />
  </button>
</template>

<style scoped>
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--s-2);
  min-height: var(--control-h);
  padding: 0 var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-base);
  font-weight: 600;
  cursor: pointer;
  /* ホバーは色・背景だけ（180ms） */
  transition:
    background-color var(--motion-base) var(--ease-out),
    border-color var(--motion-base) var(--ease-out);
  white-space: nowrap;
}

.btn-sm {
  min-height: var(--control-h-sm);
  padding: 0 var(--s-3);
  font-size: var(--font-size-sm);
}

.btn:hover:not(:disabled) {
  background: var(--accent-soft);
}

/* 無効（押せない）ボタンだけを薄くする（opacity .4）。送信中など loading のボタンは文字を読ませたいので、色はそのままにする。 */
.btn:disabled:not([aria-busy='true']) {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn[aria-busy='true'] {
  cursor: progress;
}

.btn-primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bone);
}

.btn-primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent) 85%, var(--bone));
}

.btn-danger {
  background: transparent;
  border-color: var(--danger-border);
  color: var(--danger);
}

.btn-danger:hover:not(:disabled) {
  background: var(--danger-soft);
}

.btn-ghost {
  background: transparent;
  border-color: transparent;
}
</style>
