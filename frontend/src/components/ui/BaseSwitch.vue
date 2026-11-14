<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'

defineProps<{ modelValue: boolean; ariaLabel: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <SwitchRoot
    class="sw"
    :model-value="modelValue"
    :aria-label="ariaLabel"
    :disabled="disabled"
    @update:model-value="(v) => emit('update:modelValue', v === true)"
  >
    <SwitchThumb class="sw-thumb" />
  </SwitchRoot>
</template>

<style scoped>
.sw {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--accent-soft);
  cursor: pointer;
  transition:
    background-color var(--motion-base) var(--ease-out),
    border-color var(--motion-base) var(--ease-out);
}

.sw:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.sw:focus-visible {
  outline: 3px solid var(--focus);
  outline-offset: 2px;
}

.sw[data-state='checked'] {
  background: var(--accent);
  border-color: var(--accent);
}

.sw-thumb {
  display: block;
  box-sizing: border-box;
  width: 14px;
  height: 14px;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--surface);
  transform: translateX(2px);
  transition: transform var(--motion-base) var(--ease-out);
}

.sw-thumb[data-state='checked'] {
  background: var(--bone);
  border-color: transparent;
  transform: translateX(18px);
}
</style>
