<script setup lang="ts">
import { common } from '../../lib/messages'

/** 大きさは --icon-sm / --icon-md / --icon-lg（16 / 20 / 24）。ガイドの既定は 20px（md）。 */
withDefaults(defineProps<{ size?: 'sm' | 'md' | 'lg' }>(), { size: 'md' })
</script>

<template>
  <span class="spinner" :class="`spinner-${size}`" role="status" :aria-label="common.loading" />
</template>

<style scoped>
/* 処理中の表示なので、prefers-reduced-motion でも回し続ける（止めるのは装飾的な動きだけ。ガイド M3）。 */
.spinner {
  display: inline-block;
  border: 2px solid var(--accent-soft);
  border-top-color: var(--accent);
  border-radius: var(--r-full);
  animation: spin 0.8s linear infinite;
  flex: none;
}

.spinner-sm {
  width: var(--icon-sm);
  height: var(--icon-sm);
}

.spinner-md {
  width: var(--icon-md);
  height: var(--icon-md);
}

.spinner-lg {
  width: var(--icon-lg);
  height: var(--icon-lg);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
