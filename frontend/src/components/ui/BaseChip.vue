<script setup lang="ts">
// tint を渡すと、その色は文字には使わず、地と枠だけに沈めて使う（地は --surface に --tint-bg、枠は透明に --tint-border を混ぜる）。文字は常に --ink。
defineProps<{ tint?: string }>()
</script>

<template>
  <span class="chip" :class="{ tinted: tint }" :style="tint ? { '--chip-tint': tint } : undefined">
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
  line-height: var(--line-height-xs);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chip.tinted {
  background: color-mix(in srgb, var(--chip-tint) var(--tint-bg), var(--surface));
  border-color: color-mix(in srgb, var(--chip-tint) var(--tint-border), transparent);
}
</style>
