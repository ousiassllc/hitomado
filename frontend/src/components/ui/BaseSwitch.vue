<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'

defineProps<{ modelValue: boolean; ariaLabel: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <SwitchRoot
    class="sw"
    :model-value="modelValue"
    :aria-label="ariaLabel"
    @update:model-value="(v) => emit('update:modelValue', v === true)"
  >
    <SwitchThumb class="sw-thumb" />
  </SwitchRoot>
</template>

<style scoped>
.sw {
  position: relative;
  width: 40px;
  height: 22px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: 11px;
  background: var(--color-bg-subtle);
  cursor: pointer;
  transition: background var(--transition-fast);
}

.sw[data-state='checked'] {
  background: var(--color-accent);
  border-color: var(--color-accent);
}

.sw-thumb {
  display: block;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--color-text-muted);
  transform: translateX(2px);
  transition: transform var(--transition-fast);
}

.sw-thumb[data-state='checked'] {
  background: var(--color-accent-text);
  transform: translateX(20px);
}
</style>
