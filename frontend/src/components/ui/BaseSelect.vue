<script setup lang="ts">
import {
  SelectContent,
  SelectItem,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'

export interface SelectOption {
  value: string
  label: string
}

defineProps<{
  modelValue?: string
  options: SelectOption[]
  placeholder?: string
  ariaLabel: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <SelectRoot :model-value="modelValue" @update:model-value="(v) => emit('update:modelValue', String(v))">
    <SelectTrigger class="sel" :aria-label="ariaLabel">
      <SelectValue :placeholder="placeholder" />
      <span class="sel-arrow" aria-hidden="true">▾</span>
    </SelectTrigger>
    <SelectPortal>
      <SelectContent class="sel-pop" position="popper" :side-offset="4">
        <SelectViewport>
          <SelectItem v-for="o in options" :key="o.value" :value="o.value" class="sel-item">
            <SelectItemText>{{ o.label }}</SelectItemText>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<style scoped>
.sel {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-height: 32px;
  min-width: 140px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.sel[data-placeholder] {
  color: var(--color-text-muted);
}

.sel-pop {
  min-width: var(--reka-select-trigger-width);
  max-height: 280px;
  background: var(--color-bg-elevated);
  color: var(--color-text);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-popup);
  padding: var(--space-1);
  z-index: 120;
}

.sel-item {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  cursor: pointer;
  outline: none;
}

.sel-item[data-highlighted] {
  background: var(--color-selected);
}

.sel-item[data-state='checked'] {
  font-weight: 600;
}
</style>
