<script setup lang="ts">
import { common } from '../../lib/messages'
import BaseTooltip from './BaseTooltip.vue'

// 属性（click・data-testid など）は BaseTooltip ではなくボタンへ渡す
defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    label: string
    /** 指定するとツールチップに「ラベル (キー)」を表示する（`title` は出さない） */
    shortcut?: string
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    disabled?: boolean
  }>(),
  { variant: 'ghost', disabled: false },
)
</script>

<template>
  <BaseTooltip :text="shortcut ? common.shortcutHint(label, shortcut) : undefined">
    <button
      v-bind="$attrs"
      class="icon-btn"
      :class="`icon-btn-${variant}`"
      type="button"
      :aria-label="label"
      :title="shortcut ? undefined : label"
      :disabled="disabled"
    >
      <slot />
    </button>
  </BaseTooltip>
</template>

<style scoped>
.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text);
  font-size: var(--font-size-lg);
  line-height: 1;
  cursor: pointer;
  transition: background var(--transition-fast);
}

.icon-btn:hover:not(:disabled) {
  background: var(--color-selected);
}

.icon-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.icon-btn-secondary {
  border-color: var(--color-border);
  background: var(--color-bg-elevated);
}

.icon-btn-primary {
  background: var(--color-accent);
  color: var(--color-accent-text);
}

.icon-btn-danger {
  color: var(--color-danger);
}
</style>
