<script setup lang="ts">
/** 折りたたみ。初回だけ必要な補足説明を、既定では閉じて置く（ネイティブの <details>）。 */
defineProps<{ summary: string }>()
</script>

<template>
  <details class="disclosure">
    <summary class="disclosure-summary">{{ summary }}</summary>
    <div class="disclosure-body"><slot /></div>
  </details>
</template>

<style scoped>
.disclosure-summary {
  width: fit-content;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.disclosure-summary:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.disclosure-body {
  margin-top: var(--space-2);
}
</style>
