<script setup lang="ts">
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui'

export interface MenuItem {
  key: string
  label: string
  disabled?: boolean
  danger?: boolean
}

defineProps<{ items: MenuItem[] }>()
const emit = defineEmits<{ select: [key: string] }>()
</script>

<template>
  <DropdownMenuRoot>
    <DropdownMenuTrigger as-child>
      <slot name="trigger" />
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent class="menu" :side-offset="4" align="end">
        <DropdownMenuItem
          v-for="item in items"
          :key="item.key"
          class="menu-item"
          :class="{ 'menu-danger': item.danger }"
          :disabled="item.disabled"
          @select="emit('select', item.key)"
        >
          {{ item.label }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<style scoped>
.menu {
  min-width: 160px;
  max-height: 320px;
  overflow: auto;
  padding: var(--space-1);
  background: var(--color-bg-elevated);
  color: var(--color-text);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-popup);
  z-index: 120;
}

.menu-item {
  display: flex;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  cursor: pointer;
  outline: none;
}

.menu-item[data-highlighted] {
  background: var(--color-selected);
}

.menu-item[data-disabled] {
  opacity: 0.5;
  cursor: not-allowed;
}

.menu-danger {
  color: var(--color-danger);
}
</style>
