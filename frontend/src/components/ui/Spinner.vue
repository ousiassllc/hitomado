<script setup lang="ts">
import { common } from '../../lib/messages'

/** 大きさは常に --icon-md（20px。ガイド O12）。 */
</script>

<template>
  <span class="spinner" role="status" :aria-label="common.loading" />
</template>

<style scoped>
/* 周期は 0.9s linear。prefers-reduced-motion: reduce では回転を止める（animation: none）。track と先端（--accent）の 2 色のリングはそのまま残るので、静止した状態でも処理中の表示だとわかる。 */
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

@media (prefers-reduced-motion: reduce) {
  .spinner {
    animation: none;
  }
}
</style>
