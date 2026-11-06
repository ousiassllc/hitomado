<script setup lang="ts">
import { common } from '../../lib/messages'
import { vRipple } from '../../lib/ripple'
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
    /** 処理中。押せなくするが、中のスピナーを読ませるので opacity は掛けない（BaseButton の loading と同じ） */
    loading?: boolean
  }>(),
  { variant: 'ghost', disabled: false, loading: false },
)
</script>

<template>
  <BaseTooltip :text="shortcut ? common.shortcutHint(label, shortcut) : undefined">
    <button
      v-bind="$attrs"
      v-ripple
      class="icon-btn"
      :class="`icon-btn-${variant}`"
      type="button"
      :aria-label="label"
      :title="shortcut ? undefined : label"
      :disabled="disabled || loading"
      :aria-busy="loading || undefined"
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
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--r-md);
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-base);
  line-height: 1;
  cursor: pointer;
  /* ホバーは色・背景だけ（180ms） */
  transition: background-color var(--motion-base) var(--ease-out);
}

.icon-btn:hover:not(:disabled) {
  background: var(--accent-soft);
}

.icon-btn:disabled:not([aria-busy='true']) {
  opacity: 0.4;
  cursor: not-allowed;
}

.icon-btn[aria-busy='true'] {
  cursor: progress;
}

.icon-btn-secondary {
  border-color: var(--border);
  background: var(--surface);
}

.icon-btn-primary {
  background: var(--accent);
  color: var(--bone);
}

.icon-btn-primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent) 85%, var(--bone));
}

.icon-btn-danger {
  color: var(--danger);
}
</style>
