<script setup lang="ts">
import { common } from '../../lib/messages'

withDefaults(defineProps<{ size?: number }>(), { size: 16 })
</script>

<template>
  <span class="spinner" role="status" :aria-label="common.loading" :style="{ width: `${size}px`, height: `${size}px` }" />
</template>

<style scoped>
.spinner {
  display: inline-block;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-accent);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex: none;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinner {
    animation: none;
    border-top-color: var(--color-text-muted);
  }
}
</style>
