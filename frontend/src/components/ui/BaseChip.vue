<script setup lang="ts">
defineProps<{ bg?: string; fg?: string }>()
</script>

<template>
  <span class="chip" :style="{ background: bg, color: fg }">
    <slot />
  </span>
</template>

<style scoped>
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--s-1);
  max-width: 100%;
  padding: 0 var(--s-2);
  border-radius: var(--r-sm);
  background: var(--bone);
  color: var(--ink);
  border: 1px solid var(--border);
  font-size: var(--font-size-xs);
  line-height: 18px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
