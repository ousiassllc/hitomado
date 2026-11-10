<script setup lang="ts">
import { common } from '../../lib/messages'

/** 大きさは常に --icon-md（20px。ガイド O12）。 */
</script>

<template>
  <span class="spinner" role="status" :aria-label="common.loading" />
</template>

<style scoped>
/* 処理中の表示なので、prefers-reduced-motion でも回し続ける（止めるのは装飾的な動きだけ。ガイド M3）。 */
.spinner {
  display: inline-block;
  border: 2px solid var(--accent-soft);
  border-top-color: var(--accent);
  border-radius: var(--r-full);
  animation: spin 0.9s linear infinite;
  flex: none;
  width: var(--icon-md);
  height: var(--icon-md);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
